package main

import (
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	logger := &SafeLogger{Logger: log.Default()}
	r := chi.NewRouter()
	r.Get("/scrape", func(w http.ResponseWriter, r *http.Request) {
		scrapeHandler(w, r, logger)
	})

	server := &http.Server{
		Addr:    ":4000",
		Handler: r,
	}

	logger.Printf("Starting server on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		logger.Printf("Server error: %v", err)
	}
}

// SafeLogger wraps a logger with a mutex for thread-safe logging.
type SafeLogger struct {
	*log.Logger
	mu sync.Mutex
}

// Printf logs a formatted message thread-safely.
func (sl *SafeLogger) Printf(format string, v ...interface{}) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.Logger.Printf(format, v...)
}
