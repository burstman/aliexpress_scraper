package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// SearchAliExpress scrapes AliExpress for products matching the query.
func SearchAliExpress(query string, attempt int) ([]Product, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath("/usr/bin/google-chrome"),
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("start-maximized", true),
		chromedp.Flag("lang", "en-US"),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	attemptCtx, attemptCancel := context.WithTimeout(ctx, 600*time.Second)
	defer attemptCancel()

	log.Printf("Attempt %d: Searching for '%s'", attempt, query)
	var screenshot []byte
	var products []Product
	var networkLogs []string
	var cookiesLoaded bool

	// Load cookies
	cookieName := "base"
	cookiesFile := fmt.Sprintf("cookies_%s.json", strings.ReplaceAll(cookieName, " ", "_"))
	if cookies, err := LoadCookies(cookiesFile); err == nil {
		log.Printf("Loading cookies from %s", cookiesFile)
		err = chromedp.Run(attemptCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				for _, cookie := range cookies {
					expires := cdp.TimeSinceEpoch(time.Unix(int64(cookie.Expires), 0))
					err := network.SetCookie(cookie.Name, cookie.Value).
						WithDomain(cookie.Domain).
						WithPath(cookie.Path).
						WithExpires(&expires).
						WithSecure(cookie.Secure).
						WithHTTPOnly(cookie.HTTPOnly).
						Do(ctx)
					if err != nil {
						log.Printf("Failed to set cookie %s: %v", cookie.Name, err)
					}
				}
				return nil
			}))
		if err != nil {
			log.Printf("Failed to load cookies: %v", err)
		} else {
			log.Println("Cookies loaded successfully")
			cookiesLoaded = true
		}
	} else {
		log.Printf("No cookies file found or error loading %s: %v", cookiesFile, err)
	}

	err := chromedp.Run(attemptCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.ListenTarget(ctx, func(ev interface{}) {
				if req, ok := ev.(*network.EventRequestWillBeSent); ok {
					networkLogs = append(networkLogs, fmt.Sprintf("Request: %s %s", req.Request.Method, req.Request.URL))
				}
				if res, ok := ev.(*network.EventResponseReceived); ok {
					networkLogs = append(networkLogs, fmt.Sprintf("Response: %s %d", res.Response.URL, res.Response.Status))
				}
			})
			return nil
		}),
		chromedp.EmulateViewport(1920, 1080),
		network.SetExtraHTTPHeaders(map[string]interface{}{
			"Accept-Language": "en-US,en;q=0.9",
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		}),
		chromedp.Navigate(`https://www.aliexpress.com/?language=en-US®ion=US`),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Sleep(time.Duration(3+rand.Float64()*3)*time.Second),

		chromedp.ActionFunc(func(ctx context.Context) error {
			var popupVisible bool
			if err := chromedp.Evaluate(`document.querySelector('div.Sk1_X._1-SOk') !== null`, &popupVisible).Do(ctx); err != nil {
				return fmt.Errorf("popup check failed: %v", err)
			}
			if popupVisible {
				log.Println("Dismissing popup...")
				if err := chromedp.Click(`div.Sk1_X._1-SOk`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("failed to click popup: %v", err)
				}
				chromedp.Sleep(time.Duration(1+rand.Float64()*2) * time.Second).Do(ctx)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			if cookiesLoaded {
				log.Println("Cookies loaded, skipping language modification")
				return nil
			}
			var flagVisible bool
			if err := chromedp.Evaluate(`document.querySelector('span.ship-to--cssFlag--3qFf5C9.country-flag-y2023.TN') !== null`, &flagVisible).Do(ctx); err != nil {
				return fmt.Errorf("flag check failed: %v", err)
			}
			if flagVisible {
				log.Println("Switching to English...")
				if err := chromedp.Click(`span.ship-to--cssFlag--3qFf5C9.country-flag-y2023.TN`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("failed to click flag: %v", err)
				}
				if err := chromedp.WaitVisible(`div.ship-to--menuItem--WdBDsYl.ship-to--newMenuItem--2Rw-XvE`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("dropdown menu not visible: %v", err)
				}
				if err := chromedp.Click(`div.select--text--1b85oDo`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("failed to click language selector: %v", err)
				}
				chromedp.Sleep(time.Duration(2+rand.Float64()*3) * time.Second).Do(ctx)
				var englishSelected bool
				if err := chromedp.Evaluate(`
					var options = Array.from(document.querySelectorAll('div.select--item--32FADYB'));
					var englishOption = options.find(el => el.textContent.trim() === 'English' || el.getAttribute('data-value') === 'en');
					if (englishOption) englishOption.click();
					englishOption !== null;
				`, &englishSelected).Do(ctx); err != nil {
					return fmt.Errorf("failed to select English option: %v", err)
				}
				if !englishSelected {
					return fmt.Errorf("English option not found in dropdown")
				}
				if err := chromedp.WaitVisible(`div.es--saveBtn--w8EuBuy`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("validate button not visible: %v", err)
				}
				if err := chromedp.Click(`div.es--saveBtn--w8EuBuy`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("failed to click validate button: %v", err)
				}
				chromedp.Sleep(time.Duration(10+rand.Float64()*5) * time.Second).Do(ctx)
				var pageLang string
				if err := chromedp.Evaluate(`document.documentElement.lang`, &pageLang).Do(ctx); err != nil {
					return fmt.Errorf("failed to check page language: %v", err)
				}
				log.Printf("Page language: %s", pageLang)
				if pageLang != "en" && pageLang != "en-US" {
					return fmt.Errorf("page language is %s, expected en or en-US", pageLang)
				}
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.WaitVisible(`input.search--keyword--15P08Ji`, chromedp.ByQuery).Do(ctx); err != nil {
				return fmt.Errorf("search input not visible: %v", err)
			}
			if err := chromedp.SendKeys(`input.search--keyword--15P08Ji`, query).Do(ctx); err != nil {
				return fmt.Errorf("failed to send query: %v", err)
			}
			if err := chromedp.Click(`input.search--submit--2VTbd-T`, chromedp.ByQuery).Do(ctx); err != nil {
				return fmt.Errorf("failed to click search button: %v", err)
			}
			chromedp.Sleep(time.Duration(20+rand.Float64()*10) * time.Second).Do(ctx)
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
				log.Printf("Failed to take pre-orders screenshot: %v", err)
			} else if err := WriteFile(fmt.Sprintf("screenshot_before_orders_%d.png", attempt), screenshot, 0644); err != nil {
				log.Printf("Failed to save pre-orders screenshot: %v", err)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			const selector = `div.eo_bp[ae_button_type="sort by"][ae_object_value="number_of_orders"]`
			var buttonExists bool
			if err := chromedp.Evaluate(`document.querySelector('`+selector+`') !== null`, &buttonExists).Do(ctx); err != nil {
				return fmt.Errorf("Orders button check failed: %v", err)
			}
			if !buttonExists {
				var fallbackExists bool
				if err := chromedp.Evaluate(`
					var btn = Array.from(document.querySelectorAll('div.eo_bp')).find(el => el.textContent.trim() === 'Orders');
					[btn !== null, btn ? btn.click() : null];
				`, &[]interface{}{&fallbackExists, nil}).Do(ctx); err != nil {
					return fmt.Errorf("fallback Orders button click failed: %v", err)
				}
				if !fallbackExists {
					return fmt.Errorf("Orders button not found")
				}
			} else {
				if err := chromedp.Evaluate(`document.querySelector('`+selector+`').click()`, nil).Do(ctx); err != nil {
					return fmt.Errorf("failed to click Orders button: %v", err)
				}
			}
			log.Println("Sorted by orders")
			chromedp.Sleep(time.Duration(10+rand.Float64()*15) * time.Second).Do(ctx)
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Scrolling to load products...")
			var prevCardCount, prevScrollHeight int
			for i := 0; i < 25; i++ {
				var scrollHeight int
				if err := chromedp.Evaluate(`
					window.scrollBy(0, window.innerHeight * (Math.random() * 0.5 + 0.5));
					document.documentElement.scrollHeight;
				`, &scrollHeight).Do(ctx); err != nil {
					log.Printf("Scroll %d failed: %v", i+1, err)
				}
				var cardCount int
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.search-card-item').length`, &cardCount).Do(ctx); err != nil {
					log.Printf("Scroll %d card count check failed: %v", i+1, err)
				}
				log.Printf("Scroll %d: height=%d, cards=%d", i+1, scrollHeight, cardCount)
				if cardCount >= 48 {
					log.Println("Target card count reached")
					break
				}
				if cardCount <= prevCardCount && scrollHeight <= prevScrollHeight && i > 2 {
					log.Println("No new cards, refreshing scroll...")
					if err := chromedp.Evaluate(`
						window.scrollTo(0, 0);
						window.scrollBy(0, document.body.scrollHeight);
						document.documentElement.scrollHeight;
					`, &scrollHeight).Do(ctx); err != nil {
						log.Printf("Scroll refresh failed: %v", err)
					}
					log.Printf("After refresh: height=%d", scrollHeight)
				}
				prevCardCount = cardCount
				prevScrollHeight = scrollHeight
				chromedp.Sleep(time.Duration(10+rand.Float64()*10) * time.Second).Do(ctx)
			}
			if err := chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil).Do(ctx); err != nil {
				log.Printf("First full-page scroll failed: %v", err)
			}
			chromedp.Sleep(time.Duration(5+rand.Float64()*5) * time.Second).Do(ctx)
			if err := chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil).Do(ctx); err != nil {
				log.Printf("Second full-page scroll failed: %v", err)
			}
			chromedp.Sleep(time.Duration(5+rand.Float64()*5) * time.Second).Do(ctx)
			if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
				log.Printf("Failed to take post-scroll screenshot: %v", err)
			} else if err := WriteFile(fmt.Sprintf("screenshot_after_scroll_%d.png", attempt), screenshot, 0644); err != nil {
				log.Printf("Failed to save post-scroll screenshot: %v", err)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Simulating human behavior...")
			if err := chromedp.Evaluate(`
				const moveMouse = (x, y) => {
					const evt = new MouseEvent('mousemove', { clientX: x, clientY: y, bubbles: true });
					document.dispatchEvent(evt);
				};
				const hoverRandom = () => {
					const els = document.querySelectorAll('[id="card-list"] a.search-card-item');
					const el = els[Math.floor(Math.random() * Math.min(els.length, 5))];
					if (el) {
						const rect = el.getBoundingClientRect();
						const evt = new MouseEvent('mouseover', {
							clientX: rect.left + Math.random() * rect.width,
							clientY: rect.top + Math.random() * rect.height,
							bubbles: true
						});
						el.dispatchEvent(evt);
					}
				};
				const clickRandom = () => {
					const els = document.querySelectorAll('[id="card-list"] a.search-card-item');
					const el = els[Math.floor(Math.random() * Math.min(els.length, 5))];
					if (el) {
						const rect = el.getBoundingClientRect();
						const evt = new MouseEvent('click', {
							clientX: rect.left + Math.random() * rect.width,
							clientY: rect.top + Math.random() * rect.height,
							bubbles: true
						});
						el.dispatchEvent(evt);
					}
				};
				moveMouse(500 + Math.random() * 200, 300 + Math.random() * 200);
				if (Math.random() > 0.1) hoverRandom();
				if (Math.random() > 0.5) clickRandom();
			`, nil).Do(ctx); err != nil {
				log.Printf("Human behavior simulation failed: %v", err)
			}
			chromedp.Sleep(time.Duration(2+rand.Float64()*3) * time.Second).Do(ctx)
			if err := chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight);`, nil).Do(ctx); err != nil {
				log.Printf("Post-human-behavior scroll failed: %v", err)
			}
			chromedp.Sleep(time.Duration(3+rand.Float64()*3) * time.Second).Do(ctx)
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Saving cookies...")
			var savedCookies []*network.Cookie
			if err := chromedp.ActionFunc(func(ctx context.Context) error {
				cookies, err := network.GetCookies().Do(ctx)
				if err != nil {
					return fmt.Errorf("failed to get cookies: %v", err)
				}
				savedCookies = cookies
				return nil
			}).Do(ctx); err != nil {
				log.Printf("Failed to retrieve cookies: %v", err)
			} else if err := SaveCookies(cookiesFile, savedCookies); err != nil {
				log.Printf("Failed to save cookies to %s: %v", cookiesFile, err)
			} else {
				log.Printf("Cookies saved to %s", cookiesFile)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			var nameExists, priceExists, ordersExists, ratingExists, linkExists bool
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] h3, [id="card-list"] [class*="title"], [id="card-list"] [class*="kp"]') !== null`, &nameExists).Do(ctx); err != nil {
				return fmt.Errorf("name selector check failed: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] [class*="price"], [id="card-list"] [class*="kr"]') !== null`, &priceExists).Do(ctx); err != nil {
				return fmt.Errorf("price selector check failed: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] [class*="sold"], [id="card-list"] [class*="order"], [id="card-list"] [class*="j7"]') !== null`, &ordersExists).Do(ctx); err != nil {
				return fmt.Errorf("orders selector check failed: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] [class*="rating"], [id="card-list"] [class*="kf"], [id="card-list"] [class*="kx"], [id="card-list"] [class*="kv"]') !== null`, &ratingExists).Do(ctx); err != nil {
				return fmt.Errorf("rating selector check failed: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] a.search-card-item[href*="/item/"], [id="card-list"] a[href*="/item/"]') !== null`, &linkExists).Do(ctx); err != nil {
				return fmt.Errorf("link selector check failed: %v", err)
			}
			if !nameExists || !priceExists || !ordersExists || !ratingExists || !linkExists {
				log.Printf("Invalid selectors: name=%v, price=%v, orders=%v, rating=%v, link=%v", nameExists, priceExists, ordersExists, ratingExists, linkExists)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Polling for products...")
			var isReady bool
			err := chromedp.Poll(
				`(function() {
					const count = document.querySelectorAll('[id="card-list"] a.search-card-item').length;
					const primaryLinkCount = document.querySelectorAll('[id="card-list"] a.search-card-item[href*="/item/"]').length;
					const fallbackLinkCount = document.querySelectorAll('[id="card-list"] a[href*="/item/"]').length;
					const ljgLinkCount = document.querySelectorAll('[id="card-list"] a.lj_g').length;
					const captcha = document.querySelector('.captcha-container, .sliderCaptcha, .baxia-dialog, .nc_wrapper, .captcha') !== null;
					console.log('Polling: card count=' + count + ', primary links=' + primaryLinkCount + ', fallback links=' + fallbackLinkCount + ', lj_g links=' + ljgLinkCount + ', captcha=' + captcha);
					return (count >= 48 && (primaryLinkCount >= 40 || fallbackLinkCount >= 40 || ljgLinkCount >= 40)) || captcha;
				})()`,
				&isReady,
				chromedp.WithPollingTimeout(180*time.Second),
				chromedp.WithPollingInterval(3*time.Second),
			).Do(ctx)
			if err != nil {
				log.Printf("Polling failed: %v", err)
				if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
					log.Printf("Failed to take failure screenshot: %v", err)
				} else if err := WriteFile(fmt.Sprintf("screenshot_polling_failure_%d.png", attempt), screenshot, 0644); err != nil {
					log.Printf("Failed to save failure screenshot: %v", err)
				}
				if err := WriteFile(fmt.Sprintf("network_log_%d.txt", attempt), []byte(strings.Join(networkLogs, "\n")), 0644); err != nil {
					log.Printf("Failed to save network log: %v", err)
				}
				var pageHTML string
				if err := chromedp.Evaluate(`document.documentElement.outerHTML`, &pageHTML).Do(ctx); err != nil {
					log.Printf("Page HTML capture failed: %v", err)
				} else if err := WriteFile(fmt.Sprintf("debug_page_polling_%d.html", attempt), []byte(pageHTML), 0644); err != nil {
					log.Printf("Failed to save page HTML: %v", err)
				}
				var cardListHTML string
				if err := chromedp.Evaluate(`document.querySelector('[id="card-list"]') ? document.querySelector('[id="card-list"]').outerHTML : 'No card-list found'`, &cardListHTML).Do(ctx); err != nil {
					log.Printf("Card list HTML capture failed: %v", err)
				} else if err := WriteFile(fmt.Sprintf("debug_card_list_%d.html", attempt), []byte(cardListHTML), 0644); err != nil {
					log.Printf("Failed to save card list HTML: %v", err)
				}
				return fmt.Errorf("failed to detect products or CAPTCHA: %v", err)
			}
			if !isReady {
				log.Println("Polling condition not met, refreshing page...")
				if err := chromedp.Evaluate(`location.reload()`, nil).Do(ctx); err != nil {
					return fmt.Errorf("failed to refresh page: %v", err)
				}
				chromedp.Sleep(time.Duration(20+rand.Float64()*10) * time.Second).Do(ctx)
				var cardCount int
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.search-card-item').length`, &cardCount).Do(ctx); err != nil {
					return fmt.Errorf("failed to get card count after refresh: %v", err)
				}
				log.Printf("After refresh: %d cards", cardCount)
				if cardCount < 40 {
					return fmt.Errorf("insufficient product cards after refresh: %d", cardCount)
				}
			}

			var cardCount, primaryLinkCount, fallbackLinkCount, ljgLinkCount int
			if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.search-card-item').length`, &cardCount).Do(ctx); err != nil {
				return fmt.Errorf("failed to get product card count: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.search-card-item[href*="/item/"]').length`, &primaryLinkCount).Do(ctx); err != nil {
				return fmt.Errorf("failed to get primary link count: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a[href*="/item/"]').length`, &fallbackLinkCount).Do(ctx); err != nil {
				return fmt.Errorf("failed to get fallback link count: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.lj_g').length`, &ljgLinkCount).Do(ctx); err != nil {
				return fmt.Errorf("failed to get lj_g link count: %v", err)
			}
			log.Printf("Found %d product cards, %d primary links, %d fallback links, %d lj_g links", cardCount, primaryLinkCount, fallbackLinkCount, ljgLinkCount)

			var captchaVisible bool
			if err := chromedp.Evaluate(`document.querySelector('.captcha-container, .sliderCaptcha, .baxia-dialog, .nc_wrapper, .captcha') !== null`, &captchaVisible).Do(ctx); err != nil {
				return fmt.Errorf("CAPTCHA check failed: %v", err)
			}
			log.Printf("CAPTCHA visible: %v", captchaVisible)
			if captchaVisible {
				return fmt.Errorf("CAPTCHA detected")
			}
			if cardCount < 40 || (primaryLinkCount < 40 && fallbackLinkCount < 40 && ljgLinkCount < 40) {
				log.Println("Insufficient cards or links, refreshing page...")
				if err := chromedp.Evaluate(`location.reload()`, nil).Do(ctx); err != nil {
					return fmt.Errorf("failed to refresh page: %v", err)
				}
				chromedp.Sleep(time.Duration(20+rand.Float64()*10) * time.Second).Do(ctx)
				var newCardCount, newPrimaryLinkCount, newFallbackLinkCount, newLjgLinkCount int
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.search-card-item').length`, &newCardCount).Do(ctx); err != nil {
					return fmt.Errorf("failed to get card count after refresh: %v", err)
				}
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.search-card-item[href*="/item/"]').length`, &newPrimaryLinkCount).Do(ctx); err != nil {
					return fmt.Errorf("failed to get primary link count after refresh: %v", err)
				}
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a[href*="/item/"]').length`, &newFallbackLinkCount).Do(ctx); err != nil {
					return fmt.Errorf("failed to get fallback link count after refresh: %v", err)
				}
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] a.lj_g').length`, &newLjgLinkCount).Do(ctx); err != nil {
					return fmt.Errorf("failed to get lj_g link count after refresh: %v", err)
				}
				log.Printf("After refresh: %d cards, %d primary links, %d fallback links, %d lj_g links", newCardCount, newPrimaryLinkCount, newFallbackLinkCount, newLjgLinkCount)
				if newCardCount < 40 || (newPrimaryLinkCount < 40 && newFallbackLinkCount < 40 && newLjgLinkCount < 40) {
					return fmt.Errorf("insufficient product cards (%d) or links (primary=%d, fallback=%d, lj_g=%d) after refresh", newCardCount, newPrimaryLinkCount, newFallbackLinkCount, newLjgLinkCount)
				}
				cardCount = newCardCount
				primaryLinkCount = newPrimaryLinkCount
				fallbackLinkCount = newFallbackLinkCount
				ljgLinkCount = newLjgLinkCount
			}
			chromedp.Sleep(5 * time.Second) // Ensure ratings and links load
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Extracting product details...")
			var rawProducts []map[string]interface{}
			var debugHTML []string
			if err := chromedp.Evaluate(`
				Array.from(document.querySelectorAll('[id="card-list"] a.search-card-item')).slice(0, 60).map(el => {
					const ratingContainer = el.querySelector('[class*="kx"], [class*="kv"]');
					const ratingEl = ratingContainer ? ratingContainer.querySelector('[class*="kf"], span, div') : null;
					const starContainer = ratingContainer || el.querySelector('[class*="kx"]');
					let rating = '';
					if (ratingEl && ratingEl.textContent.trim()) {
						const text = ratingEl.textContent.trim();
						if (/^\d*\.?\d+$/.test(text)) {
							rating = text;
						}
					}
					if (!rating && starContainer) {
						const stars = starContainer.querySelectorAll('img').length;
						if (stars > 0) {
							rating = stars.toString() + '.0';
						}
					}
					const linkEl = el.href.includes('/item/') ? el : el.querySelector('a[href*="/item/"]');
					const fallbackLinkEl = !linkEl && el.querySelector('a.lj_g') ? el.querySelector('a.lj_g') : null;
					const result = {
						name: el.querySelector('h3, [class*="title"], [class*="kp"]') ? el.querySelector('h3, [class*="title"], [class*="kp"]').textContent.trim() : '',
						price: el.querySelector('[class*="price"], [class*="kr"]') ? el.querySelector('[class*="price"], [class*="kr"]').textContent.trim() : '',
						orders: el.querySelector('[class*="sold"], [class*="order"], [class*="j7"]') ? el.querySelector('[class*="sold"], [class*="order"], [class*="j7"]').textContent.trim() : '',
						rating: rating,
						link: linkEl ? linkEl.href : (fallbackLinkEl ? fallbackLinkEl.href : ''),
						html: (!rating || !linkEl) ? el.outerHTML : ''
					};
					return result;
				}).filter(p => p.name && p.price && p.link);
			`, &rawProducts).Do(ctx); err != nil {
				log.Printf("Primary extraction failed: %v", err)
				// Try fallback selector
				if err := chromedp.Evaluate(`
					Array.from(document.querySelectorAll('[id="card-list"] a[href*="/item/"]')).slice(0, 60).map(el => {
						const ratingContainer = el.querySelector('[class*="kx"], [class*="kv"]');
						const ratingEl = ratingContainer ? ratingContainer.querySelector('[class*="kf"], span, div') : null;
						const starContainer = ratingContainer || el.querySelector('[class*="kx"]');
						let rating = '';
						if (ratingEl && ratingEl.textContent.trim()) {
							const text = ratingEl.textContent.trim();
							if (/^\d*\.?\d+$/.test(text)) {
								rating = text;
							}
						}
						if (!rating && starContainer) {
							const stars = starContainer.querySelectorAll('img').length;
							if (stars > 0) {
								rating = stars.toString() + '.0';
							}
						}
						const result = {
							name: el.querySelector('h3, [class*="title"], [class*="kp"]') ? el.querySelector('h3, [class*="title"], [class*="kp"]').textContent.trim() : '',
							price: el.querySelector('[class*="price"], [class*="kr"]') ? el.querySelector('[class*="price"], [class*="kr"]').textContent.trim() : '',
							orders: el.querySelector('[class*="sold"], [class*="order"], [class*="j7"]') ? el.querySelector('[class*="sold"], [class*="order"], [class*="j7"]').textContent.trim() : '',
							rating: rating,
							link: el.href,
							html: (!rating || !el.href) ? el.outerHTML : ''
						};
						return result;
					}).filter(p => p.name && p.price && p.link);
				`, &rawProducts).Do(ctx); err != nil {
					return fmt.Errorf("fallback extraction failed: %v", err)
				}
			}

			ordersRegex := regexp.MustCompile(`^(\d{1,3}(,\d{3})*|\d{4})\+?\s*sold$`)
			ordersCount := 0
			missingRatings := 0
			missingLinks := 0
			for _, p := range rawProducts {
				link, _ := p["link"].(string)
				if link == "" {
					missingLinks++
					log.Printf("Missing link for product: %s", p["name"].(string))
				}
				product := Product{
					Name:  p["name"].(string),
					Price: p["price"].(string),
					Link:  link,
				}

				ordersStr := p["orders"].(string)
				if ordersStr != "" {
					matches := ordersRegex.FindStringSubmatch(ordersStr)
					if len(matches) > 1 {
						cleanOrders := strings.ReplaceAll(matches[1], ",", "")
						orders, err := strconv.Atoi(cleanOrders)
						if err != nil {
							log.Printf("Failed to parse orders '%s': %v", ordersStr, err)
						} else {
							product.Orders = orders
							ordersCount++
						}
					}
				}

				ratingStr := p["rating"].(string)
				if ratingStr != "" {
					rating, err := strconv.ParseFloat(ratingStr, 64)
					if err != nil {
						log.Printf("Failed to parse rating '%s' for product '%s': %v", ratingStr, product.Name, err)
					} else {
						product.Rating = &rating
					}
				} else {
					missingRatings++
					log.Printf("Missing rating for product: %s", product.Name)
					zero := 0.0
					product.Rating = &zero
				}

				if ratingStr == "" || link == "" {
					if html, ok := p["html"].(string); ok && html != "" {
						debugHTML = append(debugHTML, fmt.Sprintf("Product: %s\nHTML: %s\n", product.Name, html))
					}
				}

				products = append(products, product)
			}
			log.Printf("Parsed orders for %d products", ordersCount)
			if missingRatings > 0 {
				log.Printf("Missing ratings for %d products (set to 0.0)", missingRatings)
			}
			if missingLinks > 0 {
				log.Printf("Missing links for %d products", missingLinks)
			}
			if missingRatings > 0 || missingLinks > 0 {
				log.Printf("Check debug_extract_%d.html for missing ratings or links", attempt)
				if err := WriteFile(fmt.Sprintf("debug_extract_%d.html", attempt), []byte(strings.Join(debugHTML, "\n---\n")), 0644); err != nil {
					log.Printf("Failed to save debug HTML: %v", err)
				}
			}
			log.Printf("Extracted %d products", len(products))

			if err := WriteFile(fmt.Sprintf("network_log_%d.txt", attempt), []byte(strings.Join(networkLogs, "\n")), 0644); err != nil {
				log.Printf("Failed to save network log: %v", err)
			}
			var cardListHTML string
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"]') ? document.querySelector('[id="card-list"]').outerHTML : 'No card-list found'`, &cardListHTML).Do(ctx); err != nil {
				log.Printf("Card list HTML capture failed: %v", err)
			} else if err := WriteFile(fmt.Sprintf("debug_card_list_%d.html", attempt), []byte(cardListHTML), 0644); err != nil {
				log.Printf("Failed to save card list HTML: %v", err)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
				log.Printf("Failed to take final screenshot: %v", err)
			} else if err := WriteFile(fmt.Sprintf("screenshot_attempt_%d.png", attempt), screenshot, 0644); err != nil {
				log.Printf("Failed to save final screenshot: %v", err)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return products, nil
}
