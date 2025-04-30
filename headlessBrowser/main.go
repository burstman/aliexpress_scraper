package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

type Product struct {
	Name   string   `json:"name"`
	Price  string   `json:"price"`
	Orders int      `json:"orders"`
	Rating *float64 `json:"rating"`
}

func searchAliExpress(query string, attempt int) ([]Product, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
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
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.ClearBrowserCookies().Do(ctx)
		}),
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
			} else if err := os.WriteFile(fmt.Sprintf("screenshot_before_orders_%d.png", attempt), screenshot, 0644); err != nil {
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
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] .search-card-item').length`, &cardCount).Do(ctx); err != nil {
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
			} else if err := os.WriteFile(fmt.Sprintf("screenshot_after_scroll_%d.png", attempt), screenshot, 0644); err != nil {
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
					const els = document.querySelectorAll('[id="card-list"] .search-card-item');
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
					const els = document.querySelectorAll('[id="card-list"] .search-card-item');
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
			var nameExists, priceExists, ordersExists, ratingExists bool
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] .search-card-item h3, [id="card-list"] .search-card-item [class*="title"], [id="card-list"] .search-card-item [class*="kp"]') !== null`, &nameExists).Do(ctx); err != nil {
				return fmt.Errorf("name selector check failed: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] .search-card-item [class*="price"], [id="card-list"] .search-card-item [class*="kr"]') !== null`, &priceExists).Do(ctx); err != nil {
				return fmt.Errorf("price selector check failed: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] .search-card-item [class*="sold"], [id="card-list"] .search-card-item [class*="order"], [id="card-list"] .search-card-item [class*="j7"]') !== null`, &ordersExists).Do(ctx); err != nil {
				return fmt.Errorf("orders selector check failed: %v", err)
			}
			if err := chromedp.Evaluate(`document.querySelector('[id="card-list"] .search-card-item [class*="rating"], [id="card-list"] .search-card-item [class*="kf"], [id="card-list"] .search-card-item [class*="kx"], [id="card-list"] .search-card-item [class*="kv"]') !== null`, &ratingExists).Do(ctx); err != nil {
				return fmt.Errorf("rating selector check failed: %v", err)
			}
			if !nameExists || !priceExists || !ordersExists || !ratingExists {
				log.Printf("Invalid selectors: name=%v, price=%v, orders=%v, rating=%v", nameExists, priceExists, ordersExists, ratingExists)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Polling for products...")
			var isReady bool
			err := chromedp.Poll(
				`(function() {
					const count = document.querySelectorAll('[id="card-list"] .search-card-item').length;
					const captcha = document.querySelector('.captcha-container, .sliderCaptcha, .baxia-dialog, .nc_wrapper, .captcha') !== null;
					console.log('Polling: card count=' + count + ', captcha=' + captcha);
					return count >= 48 || captcha;
				})()`,
				&isReady,
				chromedp.WithPollingTimeout(150*time.Second),
				chromedp.WithPollingInterval(3*time.Second),
			).Do(ctx)
			if err != nil {
				log.Printf("Polling failed: %v", err)
				if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
					log.Printf("Failed to take failure screenshot: %v", err)
				} else if err := os.WriteFile(fmt.Sprintf("screenshot_polling_failure_%d.png", attempt), screenshot, 0644); err != nil {
					log.Printf("Failed to save failure screenshot: %v", err)
				}
				if err := os.WriteFile(fmt.Sprintf("network_log_%d.txt", attempt), []byte(strings.Join(networkLogs, "\n")), 0644); err != nil {
					log.Printf("Failed to save network log: %v", err)
				}
				var pageHTML string
				if err := chromedp.Evaluate(`document.documentElement.outerHTML`, &pageHTML).Do(ctx); err != nil {
					log.Printf("Page HTML capture failed: %v", err)
				} else if err := os.WriteFile(fmt.Sprintf("debug_page_polling_%d.html", attempt), []byte(pageHTML), 0644); err != nil {
					log.Printf("Failed to save page HTML: %v", err)
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
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] .search-card-item').length`, &cardCount).Do(ctx); err != nil {
					return fmt.Errorf("failed to get card count after refresh: %v", err)
				}
				log.Printf("After refresh: %d cards", cardCount)
				if cardCount < 40 {
					return fmt.Errorf("insufficient product cards after refresh: %d", cardCount)
				}
			}

			var cardCount int
			if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] .search-card-item').length`, &cardCount).Do(ctx); err != nil {
				return fmt.Errorf("failed to get product card count: %v", err)
			}
			log.Printf("Found %d product cards", cardCount)

			var captchaVisible bool
			if err := chromedp.Evaluate(`document.querySelector('.captcha-container, .sliderCaptcha, .baxia-dialog, .nc_wrapper, .captcha') !== null`, &captchaVisible).Do(ctx); err != nil {
				return fmt.Errorf("CAPTCHA check failed: %v", err)
			}
			log.Printf("CAPTCHA visible: %v", captchaVisible)
			if captchaVisible {
				return fmt.Errorf("CAPTCHA detected")
			}
			if cardCount < 40 {
				log.Println("Insufficient cards, refreshing page...")
				if err := chromedp.Evaluate(`location.reload()`, nil).Do(ctx); err != nil {
					return fmt.Errorf("failed to refresh page: %v", err)
				}
				chromedp.Sleep(time.Duration(20+rand.Float64()*10) * time.Second).Do(ctx)
				var newCardCount int
				if err := chromedp.Evaluate(`document.querySelectorAll('[id="card-list"] .search-card-item').length`, &newCardCount).Do(ctx); err != nil {
					return fmt.Errorf("failed to get card count after refresh: %v", err)
				}
				log.Printf("After refresh: %d cards", newCardCount)
				if newCardCount < 40 {
					return fmt.Errorf("insufficient product cards after refresh: %d", newCardCount)
				}
				cardCount = newCardCount
			}
			chromedp.Sleep(5 * time.Second) // Ensure ratings load
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Println("Extracting product details...")
			var rawProducts []map[string]interface{}
			var debugHTML []string
			if err := chromedp.Evaluate(`
				Array.from(document.querySelectorAll('[id="card-list"] .search-card-item')).slice(0, 20).map(el => {
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
						html: !rating ? el.outerHTML : ''
					};
					return result;
				}).filter(p => p.name && p.price);
			`, &rawProducts).Do(ctx); err != nil {
				return fmt.Errorf("failed to extract product data: %v", err)
			}

			ordersRegex := regexp.MustCompile(`^(\d{1,3}(,\d{3})*|\d{4})\+?\s*sold$`)
			ordersCount := 0
			missingRatings := 0
			for _, p := range rawProducts {
				product := Product{
					Name:  p["name"].(string),
					Price: p["price"].(string),
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
					if html, ok := p["html"].(string); ok && html != "" {
						debugHTML = append(debugHTML, fmt.Sprintf("Product: %s\nHTML: %s\n", product.Name, html))
					}
				}

				products = append(products, product)
			}
			log.Printf("Parsed orders for %d products", ordersCount)
			if missingRatings > 0 {
				log.Printf("Missing ratings for %d products (set to 0.0, check debug_ratings_%d.html)", missingRatings, attempt)
				if err := os.WriteFile(fmt.Sprintf("debug_ratings_%d.html", attempt), []byte(strings.Join(debugHTML, "\n---\n")), 0644); err != nil {
					log.Printf("Failed to save debug HTML: %v", err)
				}
			}
			log.Printf("Extracted %d products", len(products))

			if err := os.WriteFile(fmt.Sprintf("network_log_%d.txt", attempt), []byte(strings.Join(networkLogs, "\n")), 0644); err != nil {
				log.Printf("Failed to save network log: %v", err)
			}
			return nil
		}),

		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
				log.Printf("Failed to take final screenshot: %v", err)
			} else if err := os.WriteFile(fmt.Sprintf("screenshot_attempt_%d.png", attempt), screenshot, 0644); err != nil {
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

func scrapeHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "query parameter is required", http.StatusBadRequest)
		return
	}

	var products []Product
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		products, err = searchAliExpress(query, attempt)
		if err == nil && len(products) >= 10 {
			break
		}
		log.Printf("Attempt %d failed: %v", attempt, err)
		time.Sleep(time.Duration(5+rand.Float64()*5) * time.Second)
	}

	if err != nil || len(products) == 0 {
		http.Error(w, fmt.Sprintf("failed to scrape products: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(products); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	results, _ := json.MarshalIndent(products, "", "  ")
	if err := os.WriteFile("products.json", results, 0644); err != nil {
		log.Printf("Failed to save products: %v", err)
	}
	log.Println("Completed successfully! Saved products.json")
}

func main() {
	rand.Seed(time.Now().UnixNano())
	r := chi.NewRouter()
	r.Get("/scrape", scrapeHandler)

	server := &http.Server{
		Addr:    ":3000",
		Handler: r,
	}

	log.Printf("Starting server on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("Server error:", err)
	}
}
