package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
)

func main() {
	log.SetFlags(log.Lshortfile)
	// Create a memory backend instance
	mem := memory.New()

	winDuration := 3 * time.Second

	// Create a basic rate limiter with fixed window strategy
	// Allow 5 requests per minute per user
	limiter, err := ratelimit.New(
		ratelimit.WithBackend(mem),
		ratelimit.WithPrimaryStrategy(
			strategies.NewFixedWindowConfig("user").
				AddTier("default", 5, winDuration).
				Build(),
		),
		ratelimit.WithBaseKey("api"),
	)
	if err != nil {
		log.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	userID := "user123"

	fmt.Println("=== Basic Rate Limiting Example ===")
	fmt.Printf("Rate limit: 5 requests per minute for user %s\n\n", userID)

	// Simulate 8 requests to demonstrate rate limiting
	for i := 1; i <= 8; i++ {
		var results map[string]strategies.Result

		// Check if request is allowed with results
		allowed, err := limiter.Allow(
			ratelimit.WithContext(ctx),
			ratelimit.WithKey(userID),
			ratelimit.WithResult(&results),
		)
		if err != nil {
			log.Printf("Error checking rate limit: %v", err)
			continue
		}

		result := results["primary_default"]
		status := "DENIED"
		if allowed {
			status = "ALLOWED"
		}

		fmt.Printf("Request %d: %s (Remaining: %d, Reset: %v)\n",
			i, status, result.Remaining, result.Reset.Format("15:04:05"))

		if !allowed {
			fmt.Printf("Rate limit exceeded. Next reset at: %v\n\n", result.Reset.Format("15:04:05"))
		}
	}

	// Demonstrate getting statistics without consuming quota
	fmt.Println("=== Getting Statistics Without Consuming Quota ===")
	var stats map[string]strategies.Result
	stats, err = limiter.GetStats(
		ratelimit.WithContext(ctx),
		ratelimit.WithKey(userID),
	)
	if err != nil {
		log.Printf("Error getting stats: %v", err)
		return
	}

	result := stats["primary_default"]
	fmt.Printf("Current stats - Remaining: %d, Reset: %v\n",
		result.Remaining, result.Reset.Format("15:04:05"))

	// Wait for rate limit to reset
	fmt.Println("\nWaiting for rate limit to reset...")
	time.Sleep(winDuration - time.Millisecond)

	// Try another request after reset
	var results map[string]strategies.Result
	allowed, err := limiter.Allow(
		ratelimit.WithContext(ctx),
		ratelimit.WithKey(userID),
		ratelimit.WithResult(&results),
	)
	if err != nil {
		log.Printf("Error checking rate limit: %v", err)
		return
	}

	result = results["primary_default"]
	status := "DENIED"
	if allowed {
		status = "ALLOWED"
	}

	fmt.Printf("Request after reset: %s (Remaining: %d)\n", status, result.Remaining)
}
