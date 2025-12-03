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
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
)

func main() {
	// Create a memory backend instance
	mem := memory.New()

	// Create a dual-strategy rate limiter
	// Primary: Fixed window for hard limits (100 requests per hour)
	// Secondary: Token bucket for burst smoothing (10 burst, 1 req/sec refill)
	limiter, err := ratelimit.New(
		ratelimit.WithBackend(mem),
		// Primary strategy: strict rate limiting
		ratelimit.WithPrimaryStrategy(
			fixedwindow.NewConfig().
				SetKey("user").
				AddQuota("default", 100, time.Hour).
				Build(),
		),
		// Secondary strategy: burst smoother
		ratelimit.WithSecondaryStrategy(&tokenbucket.Config{
			Burst: 10,  // Allow up to 10 requests in a burst
			Rate:  1.0, // Refill at 1 token per second
		}),
		ratelimit.WithBaseKey("api"),
	)
	if err != nil {
		log.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	userID := "user456"

	fmt.Println("=== Dual Strategy Rate Limiting Example ===")
	fmt.Printf("Primary: 100 requests per hour (fixed window)\n")
	fmt.Printf("Secondary: 10 burst capacity, 1 req/sec refill (token bucket)\n")
	fmt.Printf("User: %s\n\n", userID)

	// First, demonstrate normal usage within limits
	fmt.Println("=== Normal Usage Within Limits ===")
	for i := 1; i <= 5; i++ {
		var results strategies.Results

		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
		if err != nil {
			log.Printf("Error checking rate limit: %v", err)
			continue
		}

		primaryResult := results.PrimaryDefault()
		secondaryResult := results.SecondaryDefault()

		status := "DENIED"
		if allowed {
			status = "ALLOWED"
		}

		fmt.Printf("Request %d: %s\n", i, status)
		fmt.Printf("  Primary - Remaining: %d\n", primaryResult.Remaining)
		fmt.Printf("  Secondary - Remaining: %.1f tokens\n", float64(secondaryResult.Remaining))
		fmt.Println()
	}

	// Demonstrate burst capacity
	fmt.Println("=== Testing Burst Capacity ===")
	fmt.Println("Sending 12 rapid requests to test burst limit...")

	burstAllowed := 0
	burstDenied := 0
	for i := 1; i <= 12; i++ {
		var results strategies.Results

		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
			Key:    userID,
			Result: &results,
		})
		if err != nil {
			log.Printf("Error checking rate limit: %v", err)
			continue
		}

		if allowed {
			burstAllowed++
		} else {
			burstDenied++
		}

		secondaryResult := results.SecondaryDefault()
		fmt.Printf("Burst request %d: %s (Secondary tokens: %.1f)\n",
			i, map[bool]string{true: "ALLOWED", false: "DENIED"}[allowed],
			float64(secondaryResult.Remaining))
	}

	fmt.Printf("\nBurst test results: %d allowed, %d denied\n", burstAllowed, burstDenied)

	// Wait for token bucket to refill
	fmt.Println("\n=== Waiting for Token Refill ===")
	fmt.Println("Waiting 3 seconds for token bucket to refill...")
	time.Sleep(3 * time.Second)

	// Try again after refill
	var results strategies.Results
	allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
		Key:    userID,
		Result: &results,
	})
	if err != nil {
		log.Printf("Error checking rate limit: %v", err)
		return
	}

	primaryResult := results.PrimaryDefault()
	secondaryResult := results.SecondaryDefault()

	status := "DENIED"
	if allowed {
		status = "ALLOWED"
	}

	fmt.Printf("Request after refill: %s\n", status)
	fmt.Printf("Primary - Remaining: %d\n", primaryResult.Remaining)
	fmt.Printf("Secondary - Remaining: %.1f tokens\n", float64(secondaryResult.Remaining))

	// Show strategy behavior over time
	fmt.Println("\n=== Strategy Behavior Over Time ===")
	fmt.Println("Sending requests every 2 seconds to observe token refill...")

	for i := 1; i <= 4; i++ {
		if i > 1 {
			time.Sleep(2 * time.Second)
		}

		var results strategies.Results
		allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
			Key:    userID,
			Result: &results,
		})
		if err != nil {
			log.Printf("Error checking rate limit: %v", err)
			continue
		}

		primaryResult := results.PrimaryDefault()
		secondaryResult := results.SecondaryDefault()

		status := "DENIED"
		if allowed {
			status = "ALLOWED"
		}

		fmt.Printf("Request %d (after %ds): %s\n", i, (i-1)*2, status)
		fmt.Printf("  Primary - Remaining: %d\n", primaryResult.Remaining)
		fmt.Printf("  Secondary - Remaining: %.1f tokens\n", float64(secondaryResult.Remaining))
	}

	fmt.Println("\n=== Summary ===")
	fmt.Println("Dual strategy provides:")
	fmt.Println("- Primary strategy: Hard limits (100/hour)")
	fmt.Println("- Secondary strategy: Burst control and smooth request distribution")
	fmt.Println("- Both strategies must allow for request to be accepted")
}
