package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type Product struct {
	Name  string `json:"name"`
	Price string `json:"price"`
}

func searchAliExpress(query string) ([]Product, error) {
	// Configure ChromeDP options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // Visible for debugging
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

	// Retry logic: attempt up to 3 times
	var products []Product
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		// Set a 240-second timeout per attempt
		attemptCtx, attemptCancel := context.WithTimeout(ctx, 240*time.Second)
		defer attemptCancel()

		log.Printf("Attempt %d: Searching for '%s'", attempt, query)
		var screenshot []byte
		err := chromedp.Run(attemptCtx,
			chromedp.EmulateViewport(1920, 1080),
			// Clear cookies
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Clearing cookies...")
				if err := network.ClearBrowserCookies().Do(ctx); err != nil {
					return fmt.Errorf("failed to clear cookies: %v", err)
				}
				return nil
			}),
			// Set Accept-Language header
			network.SetExtraHTTPHeaders(map[string]interface{}{
				"Accept-Language": "en-US,en;q=0.9",
			}),
			chromedp.Navigate(`https://www.aliexpress.com/?language=en-US®ion=US`),
			chromedp.WaitVisible(`body`, chromedp.ByQuery),
			chromedp.Sleep(2*time.Second), // Wait for potential popups

			// Handle popup if present
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Checking for popups...")
				var popupVisible bool
				if err := chromedp.Evaluate(`document.querySelector('div.Sk1_X._1-SOk') !== null`, &popupVisible).Do(ctx); err != nil {
					return fmt.Errorf("popup check failed: %v", err)
				}
				if popupVisible {
					log.Println("Popup dismiss button found, clicking...")
					if err := chromedp.Click(`div.Sk1_X._1-SOk`, chromedp.ByQuery).Do(ctx); err != nil {
						return fmt.Errorf("failed to click popup: %v", err)
					}
					chromedp.Sleep(1 * time.Second).Do(ctx)
				}
				return nil
			}),

			// Handle language/region selection
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Checking for Tunisian flag...")
				var flagVisible bool
				if err := chromedp.Evaluate(`document.querySelector('span.ship-to--cssFlag--3qFf5C9.country-flag-y2023.TN') !== null`, &flagVisible).Do(ctx); err != nil {
					return fmt.Errorf("flag check failed: %v", err)
				}
				if flagVisible {
					log.Println("Tunisian flag found, clicking...")
					if err := chromedp.Click(`span.ship-to--cssFlag--3qFf5C9.country-flag-y2023.TN`, chromedp.ByQuery).Do(ctx); err != nil {
						return fmt.Errorf("failed to click flag: %v", err)
					}
					if err := chromedp.WaitVisible(`div.ship-to--menuItem--WdBDsYl.ship-to--newMenuItem--2Rw-XvE`, chromedp.ByQuery).Do(ctx); err != nil {
						return fmt.Errorf("dropdown menu not visible: %v", err)
					}
					log.Println("Dropdown menu visible, selecting language...")
					if err := chromedp.Click(`div.select--text--1b85oDo`, chromedp.ByQuery).Do(ctx); err != nil {
						return fmt.Errorf("failed to click language selector: %v", err)
					}
					chromedp.Sleep(2 * time.Second).Do(ctx)
					// Select English by text or value
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
					log.Println("Validate button visible, clicking...")
					if err := chromedp.Click(`div.es--saveBtn--w8EuBuy`, chromedp.ByQuery).Do(ctx); err != nil {
						return fmt.Errorf("failed to click validate button: %v", err)
					}
					chromedp.Sleep(10 * time.Second).Do(ctx) // Wait for page reload

					// Verify language
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

			// Perform search
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Waiting for search input...")
				if err := chromedp.WaitVisible(`input.search--keyword--15P08Ji`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("search input not visible: %v", err)
				}
				log.Println("Sending query to search input...")
				if err := chromedp.SendKeys(`input.search--keyword--15P08Ji`, query).Do(ctx); err != nil {
					return fmt.Errorf("failed to send query: %v", err)
				}
				log.Println("Clicking search button...")
				if err := chromedp.Click(`input.search--submit--2VTbd-T`, chromedp.ByQuery).Do(ctx); err != nil {
					return fmt.Errorf("failed to click search button: %v", err)
				}
				log.Println("Waiting for results page...")
				chromedp.Sleep(15 * time.Second).Do(ctx) // Wait for results to load
				return nil
			}),

			// Take a screenshot before Orders button click
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Taking screenshot before Orders button click...")
				if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
					log.Printf("Error taking screenshot: %v", err)
				}
				if err := os.WriteFile(fmt.Sprintf("screenshot_before_orders_%d.png", attempt), screenshot, 0644); err != nil {
					log.Printf("Error saving screenshot: %v", err)
				}
				return nil
			}),

			// Check and click the Orders button
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Checking for Orders button...")
				var buttonExists bool
				const selector = `div.eo_bp[ae_button_type="sort by"][ae_object_value="number_of_orders"]`
				if err := chromedp.Evaluate(`document.querySelector('`+selector+`') !== null`, &buttonExists).Do(ctx); err != nil {
					return fmt.Errorf("Orders button check failed: %v", err)
				}
				log.Printf("Orders button exists: %v", buttonExists)

				if !buttonExists {
					log.Println("Attempting fallback click by text content...")
					var fallbackExists bool
					if err := chromedp.Evaluate(`
						var btn = Array.from(document.querySelectorAll('div.eo_bp')).find(el => el.textContent.trim() === 'Orders');
						[btn !== null, btn ? btn.click() : null];
					`, &[]interface{}{&fallbackExists, nil}).Do(ctx); err != nil {
						return fmt.Errorf("fallback Orders button click failed: %v", err)
					}
					log.Printf("Fallback Orders button exists: %v", fallbackExists)
					if !fallbackExists {
						return fmt.Errorf("Orders button not found in DOM")
					}
				} else {
					log.Println("Attempting to click Orders button via JavaScript...")
					if err := chromedp.Evaluate(`document.querySelector('`+selector+`').click()`, nil).Do(ctx); err != nil {
						return fmt.Errorf("failed to click Orders button via JavaScript: %v", err)
					}
				}
				log.Println("Orders button clicked successfully")
				chromedp.Sleep(45 * time.Second).Do(ctx) // Wait for page reload
				return nil
			}),

			// Incremental scroll to load products
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Performing incremental scrolling...")
				for i := 0; i < 3; i++ {
					if err := chromedp.Evaluate(`window.scrollBy(0, window.innerHeight);`, nil).Do(ctx); err != nil {
						log.Printf("Scroll %d failed: %v", i+1, err)
					}
					var jrJ4Count int
					if err := chromedp.Evaluate(`document.querySelectorAll('.jr_j4').length`, &jrJ4Count).Do(ctx); err != nil {
						log.Printf("Scroll %d jr_j4 count check failed: %v", i+1, err)
					}
					log.Printf("After scroll %d: Found %d product containers (.jr_j4)", i+1, jrJ4Count)
					chromedp.Sleep(10 * time.Second).Do(ctx) // Wait for products to load
				}
				return nil
			}),

			// Debug product containers
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Debugging product containers...")
				var jrJ4Count int
				if err := chromedp.Evaluate(`document.querySelectorAll('.jr_j4').length`, &jrJ4Count).Do(ctx); err != nil {
					log.Printf("jr_j4 count check failed: %v", err)
				}
				log.Printf("Found %d product containers (.jr_j4)", jrJ4Count)

				var sampleJrJ4HTML string
				if err := chromedp.Evaluate(`document.querySelector('.jr_j4') ? document.querySelector('.jr_j4').outerHTML : ''`, &sampleJrJ4HTML).Do(ctx); err != nil {
					log.Printf("Sample jr_j4 HTML check failed: %v", err)
				}
				log.Printf("Sample jr_j4 HTML: %s", sampleJrJ4HTML)

				var titleExists, priceExists bool
				if err := chromedp.Evaluate(`document.querySelector('.jr_j4 .jr_kp') !== null`, &titleExists).Do(ctx); err != nil {
					log.Printf("jr_kp check failed: %v", err)
				}
				if err := chromedp.Evaluate(`document.querySelector('.jr_j4 .jr_kr') !== null`, &priceExists).Do(ctx); err != nil {
					log.Printf("jr_kr check failed: %v", err)
				}
				log.Printf("Child selectors exist: jr_kp=%v, jr_kr=%v", titleExists, priceExists)
				return nil
			}),

			// Wait for results or CAPTCHA with polling
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Polling for product containers or CAPTCHA...")
				var isReady bool
				err := chromedp.Poll(
					`document.querySelectorAll('.jr_j4').length >= 40 || document.querySelector('.captcha-container, .sliderCaptcha, .baxia-dialog, .nc_wrapper, .captcha') !== null`,
					&isReady,
					chromedp.WithPollingTimeout(90*time.Second),
					chromedp.WithPollingInterval(5*time.Second),
				).Do(ctx)
				if err != nil {
					log.Printf("Polling failed: %v", err)
					return fmt.Errorf("failed to detect products or CAPTCHA: %v", err)
				}
				if !isReady {
					return fmt.Errorf("polling condition not met")
				}

				var jrJ4Count int
				if err := chromedp.Evaluate(`document.querySelectorAll('.jr_j4').length`, &jrJ4Count).Do(ctx); err != nil {
					return fmt.Errorf("failed to get product container count: %v", err)
				}
				log.Printf("Found %d product containers (.jr_j4)", jrJ4Count)

				var captchaVisible bool
				if err := chromedp.Evaluate(`document.querySelector('.captcha-container, .sliderCaptcha, .baxia-dialog, .nc_wrapper, .captcha') !== null`, &captchaVisible).Do(ctx); err != nil {
					return fmt.Errorf("CAPTCHA check failed: %v", err)
				}
				log.Printf("CAPTCHA visible: %v", captchaVisible)
				if captchaVisible {
					return fmt.Errorf("CAPTCHA detected")
				}
				if jrJ4Count < 40 {
					return fmt.Errorf("insufficient product containers found (.jr_j4): %d", jrJ4Count)
				}
				return nil
			}),

			// Extract product details
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Extracting product details...")
				var rawProducts []map[string]string
				if err := chromedp.Evaluate(`
					Array.from(document.querySelectorAll('.jr_j4')).slice(0, 20).map(el => ({
						name: el.querySelector('.jr_kp') ? el.querySelector('.jr_kp').textContent.trim() : '',
						price: el.querySelector('.jr_kr') ? el.querySelector('.jr_kr').textContent.trim() : ''
					})).filter(p => p.name && p.price);
				`, &rawProducts).Do(ctx); err != nil {
					return fmt.Errorf("failed to extract product data: %v", err)
				}
				for _, p := range rawProducts {
					products = append(products, Product{Name: p["name"], Price: p["price"]})
				}
				log.Printf("Extracted %d products", len(products))
				return nil
			}),

			// Take a final screenshot
			chromedp.ActionFunc(func(ctx context.Context) error {
				log.Println("Taking final screenshot...")
				if err := chromedp.Screenshot(`body`, &screenshot, chromedp.ByQuery).Do(ctx); err != nil {
					log.Printf("Error taking screenshot: %v", err)
				}
				if err := os.WriteFile(fmt.Sprintf("screenshot_attempt_%d.png", attempt), screenshot, 0644); err != nil {
					log.Printf("Error saving screenshot: %v", err)
				}
				return nil
			}),
		)
		if err == nil && len(products) > 0 {
			return products, nil
		}
		lastErr = err
		log.Printf("Attempt %d failed: %v", attempt, err)

		// Clean up Chrome before retry
		log.Println("Cleaning up Chrome context...")
		chromedp.Run(ctx, chromedp.Stop())
		time.Sleep(3 * time.Second) // Wait before retrying
	}
	if len(products) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("all attempts failed: %v", lastErr)
		}
		return nil, fmt.Errorf("no products found for query: %s", query)
	}
	return products, nil
}

func main() {
	query := "laptop" // Test query
	products, err := searchAliExpress(query)
	if err != nil {
		log.Fatal("Error during ChromeDP execution:", err)
	}

	// Print results
	log.Println("Extracted products:")
	for _, product := range products {
		log.Printf("Name: %s\nPrice: %s\n---", product.Name, product.Price)
	}

	// Save results to a file
	results, _ := json.MarshalIndent(products, "", "  ")
	if err := os.WriteFile("products.json", results, 0644); err != nil {
		log.Fatal("Error saving products:", err)
	}

	log.Println("Process completed successfully! Results saved as products.json, screenshot as screenshot_attempt_X.png")
}
