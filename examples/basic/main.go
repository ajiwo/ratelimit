package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ajiwo/ratelimit"
	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
)

func main() {
	log.SetFlags(log.Lshortfile)
	// Create a memory backend instance
	mem := memory.New()

	winDuration := 3 * time.Second

	// Create a basic rate limiter with fixed window strategy
	// Allow 5 requests per 3 seconds per user
	limiter, err := ratelimit.New(
		ratelimit.WithBackend(mem),
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 5, winDuration).
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
		var results strategies.Results

		// Check if request is allowed with results
		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
			Key:    userID,
			Result: &results,
		})
		if err != nil {
			log.Printf("Error checking rate limit: %v", err)
			continue
		}

		result := results["default"]
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
	var stats strategies.Results
	_, err = limiter.Peek(ctx, ratelimit.AccessOptions{
		Key:    userID,
		Result: &stats,
	})
	if err != nil {
		log.Printf("Error getting stats: %v", err)
		return
	}

	result := stats["default"]
	fmt.Printf("Current stats - Remaining: %d, Reset: %v\n",
		result.Remaining, result.Reset.Format("15:04:05"))

	// Wait for rate limit to reset
	fmt.Println("Waiting for rate limit to reset...")
	time.Sleep(time.Until(result.Reset))

	// Try another request after reset
	var results strategies.Results
	// allowed, err := limiter.Allow(ctx, &userID, &results)
	allowed, err := limiter.Peek(ctx, ratelimit.AccessOptions{
		Key:    userID,
		Result: &results,
	})
	if err != nil {
		log.Printf("Error checking rate limit: %v", err)
		return
	}

	result = results["default"]
	status := "DENIED"
	if allowed {
		status = "ALLOWED"
	}

	fmt.Printf("Request after reset: %s (Remaining: %d)\n", status, result.Remaining)
}
