package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

func scrapeHandler(w http.ResponseWriter, r *http.Request, logger *SafeLogger) {
	queriesStr := r.URL.Query().Get("queries")
	if queriesStr == "" {
		http.Error(w, "queries parameter is required", http.StatusBadRequest)
		return
	}

	queries := strings.Split(queriesStr, ",")
	if len(queries) == 0 {
		http.Error(w, "at least one query is required", http.StatusBadRequest)
		return
	}

	// Limit concurrency to 3 simultaneous scrapes
	const maxConcurrent = 3
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var fileMu sync.Mutex
	results := make(map[string][]Product)
	var resultMu sync.Mutex

	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			var products []Product
			var err error
			for attempt := 1; attempt <= 3; attempt++ {
				products, err = SearchAliExpress(q, attempt, logger, &fileMu)
				if err == nil && len(products) >= 10 {
					break
				}
				logger.Printf("Attempt %d for query '%s' failed: %v", attempt, q, err)
				time.Sleep(5 * time.Second)
			}

			if err != nil || len(products) == 0 {
				logger.Printf("Failed to scrape products for query '%s': %v", q, err)
				return
			}

			resultMu.Lock()
			results[q] = products
			resultMu.Unlock()
		}(query)
	}

	wg.Wait()

	if len(results) == 0 {
		http.Error(w, "no products scraped for any query", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	resultsJSON, _ := json.MarshalIndent(results, "", "  ")
	if err := WriteFile("products.json", resultsJSON, 0644, &fileMu); err != nil {
		logger.Printf("Failed to save products: %v", err)
	}
	logger.Println("Completed successfully! Saved products.json")
}
