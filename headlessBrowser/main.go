package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	r := chi.NewRouter()
	r.Get("/scrape", scrapeHandler)

	server := &http.Server{
		Addr:    ":4000",
		Handler: r,
	}

	log.Printf("Starting server on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("Server error:", err)
	}
}
