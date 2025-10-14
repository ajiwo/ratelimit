package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
)

func main() {
	// Create rate limiter
	limiter, err := ratelimit.New(
		ratelimit.WithBackend(memory.New()),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig("client").
				AddQuota("default", 10, time.Minute).
				Build(),
		),
		ratelimit.WithBaseKey("api"),
	)
	if err != nil {
		log.Fatalf("Failed to create rate limiter: %v", err)
	}

	// Create HTTP server with rate limiting middleware
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from protected endpoint!\n")
	})

	// Wrap with rate limiting middleware
	handler := middleware(limiter, mux)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Println("Server listening on :8080")
	if err := server.ListenAndServe(); err != nil {
		log.Printf("Server error: %v", err)
	}
	limiter.Close()
}

func middleware(limiter *ratelimit.RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := getClientID(r)

		// Check rate limit
		allowed, err := limiter.Allow(
			ratelimit.WithContext(r.Context()),
			ratelimit.WithKey(clientID),
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		log.Printf("%s allowed %t", clientID, allowed)

		if !allowed {
			// Get rate limit info for response headers
			stats, err := limiter.GetStats(
				ratelimit.WithContext(r.Context()),
				ratelimit.WithKey(clientID),
			)
			if err == nil && len(stats) > 0 {
				result := stats["default"]
				w.Header().Set("X-RateLimit-Limit", "10")
				w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
				w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.Reset.Unix()))
			}

			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Request allowed, proceed to next handler
		next.ServeHTTP(w, r)
	})
}

func getClientID(r *http.Request) string {
	// - IP address
	return strings.Split(r.RemoteAddr, ":")[0]
}
