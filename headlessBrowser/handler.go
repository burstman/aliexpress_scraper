package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

func scrapeHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "query parameter is required", http.StatusBadRequest)
		return
	}

	var products []Product
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		products, err = SearchAliExpress(query, attempt)
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
	if err := WriteFile("products.json", results, 0644); err != nil {
		log.Printf("Failed to save products: %v", err)
	}
	log.Println("Completed successfully! Saved products.json")
}
