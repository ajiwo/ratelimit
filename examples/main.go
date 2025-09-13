package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

func main() {
	// Example 1: Token Bucket with Memory Backend
	fmt.Println("=== Token Bucket with Memory Backend ===")
	if err := tokenBucketExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 2: Fixed Window with Memory Backend
	fmt.Println("\n=== Fixed Window with Memory Backend ===")
	if err := fixedWindowExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 3: Token Bucket with Redis Backend
	fmt.Println("\n=== Token Bucket with Redis Backend ===")
	if err := tokenBucketRedisExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 4: Get rate limit status
	fmt.Println("\n=== Getting Rate Limit Status ===")
	if err := statusExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func tokenBucketExample() error {
	// Create memory backend
	backendConfig := backends.BackendConfig{
		Type:            "memory",
		CleanupInterval: 5 * time.Minute,
		MaxKeys:         10000,
	}

	backend, err := backends.NewBackend(backendConfig)
	if err != nil {
		return err
	}
	defer backend.Close()

	// Create token bucket strategy
	strategyConfig := strategies.StrategyConfig{
		Type:         "token_bucket",
		RefillRate:   time.Second,
		RefillAmount: 5,
		BucketSize:   20,
	}

	strategy, err := strategies.NewStrategy(strategyConfig)
	if err != nil {
		return err
	}

	// Test rate limiting
	userID := "user:token_bucket:123"

	for i := range 25 {
		result, err := strategy.Allow(context.Background(), backend, userID, 20, time.Minute)
		if err != nil {
			return err
		}

		if result.Allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED (retry after: %v)\n", i+1, result.RetryAfter)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func fixedWindowExample() error {
	// Create memory backend
	backendConfig := backends.BackendConfig{
		Type:            "memory",
		CleanupInterval: 5 * time.Minute,
		MaxKeys:         10000,
	}

	backend, err := backends.NewBackend(backendConfig)
	if err != nil {
		return err
	}
	defer backend.Close()

	// Create fixed window strategy
	strategyConfig := strategies.StrategyConfig{
		Type:           "fixed_window",
		WindowDuration: 10 * time.Second, // Short window for demo
	}

	strategy, err := strategies.NewStrategy(strategyConfig)
	if err != nil {
		return err
	}

	// Test rate limiting
	userID := "user:fixed_window:456"

	for i := range 15 {
		result, err := strategy.Allow(context.Background(), backend, userID, 10, 10*time.Second)
		if err != nil {
			return err
		}

		if result.Allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED (retry after: %v)\n", i+1, result.RetryAfter)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func tokenBucketRedisExample() error {
	// Get Redis address from environment variable
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Default Redis address
	}

	// Create Redis backend
	backendConfig := backends.BackendConfig{
		Type:          "redis",
		RedisAddress:  redisAddr,
		RedisPassword: "", // Assuming no password for local Redis
		RedisDB:       0,  // Default DB
		RedisPoolSize: 10,
	}

	backend, err := backends.NewBackend(backendConfig)
	if err != nil {
		return err
	}
	defer backend.Close()

	// Create token bucket strategy
	strategyConfig := strategies.StrategyConfig{
		Type:         "token_bucket",
		RefillRate:   time.Second,
		RefillAmount: 3,
		BucketSize:   15,
	}

	strategy, err := strategies.NewStrategy(strategyConfig)
	if err != nil {
		return err
	}

	// Test rate limiting
	userID := "user:token_bucket:redis:789"

	for i := range 20 {
		result, err := strategy.Allow(context.Background(), backend, userID, 15, time.Minute)
		if err != nil {
			return err
		}

		if result.Allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED (retry after: %v)\n", i+1, result.RetryAfter)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func statusExample() error {
	// Create memory backend
	backendConfig := backends.BackendConfig{
		Type:            "memory",
		CleanupInterval: 5 * time.Minute,
		MaxKeys:         10000,
	}

	backend, err := backends.NewBackend(backendConfig)
	if err != nil {
		return err
	}
	defer backend.Close()

	// Create token bucket strategy
	strategyConfig := strategies.StrategyConfig{
		Type:         "token_bucket",
		RefillRate:   time.Second,
		RefillAmount: 2,
		BucketSize:   10,
	}

	strategy, err := strategies.NewStrategy(strategyConfig)
	if err != nil {
		return err
	}

	userID := "user:status:789"

	// Make some requests to consume tokens
	for i := range 5 {
		result, err := strategy.Allow(context.Background(), backend, userID, 10, time.Minute)
		if err != nil {
			return err
		}
		fmt.Printf("Made request %d: allowed=%v, remaining=%d\n", i+1, result.Allowed, result.Remaining)
		time.Sleep(200 * time.Millisecond)
	}

	// Get current status
	status, err := strategy.GetStatus(context.Background(), backend, userID, 10, time.Minute)
	if err != nil {
		return err
	}

	fmt.Printf("\nCurrent Status:\n")
	fmt.Printf("  Limit: %d\n", status.Limit)
	fmt.Printf("  Remaining: %d\n", status.Remaining)
	fmt.Printf("  Window Start: %v\n", status.WindowStart)
	fmt.Printf("  Window Duration: %v\n", status.WindowDuration)
	fmt.Printf("  Total Requests: %d\n", status.TotalRequests)
	return nil
}
