package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ajiwo/ratelimit"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/backends/postgres"
	"github.com/ajiwo/ratelimit/backends/redis"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"

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

	// Example 5: Token Bucket with PostgreSQL Backend
	fmt.Println("\n=== Token Bucket with PostgreSQL Backend ===")
	if err := tokenBucketPostgresExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	// Example 6: Dual Strategy (Fixed Window + Token Bucket)
	fmt.Println("\n=== Dual Strategy (Fixed Window + Token Bucket) ===")
	if err := dualStrategyExample(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func tokenBucketExample() error {
	// Create memory backend
	// backend, _ := backends.Create("memory", nil)
	backend := memory.New()

	// Create token bucket strategy
	strategy := tokenbucket.New(backend)

	// Test rate limiting
	userID := "user:token_bucket:123"
	config := tokenbucket.Config{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 20,
		},
		BurstSize:  20,
		RefillRate: 5.0, // 5 tokens per second
	}

	for i := range 25 {
		result, err := strategy.AllowWithResult(context.Background(), config)
		if err != nil {
			return err
		}

		if result.Allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func fixedWindowExample() error {
	// Create memory backend
	backend, _ := backends.Create("memory", nil)

	// Create fixed window strategy
	strategy := fixedwindow.New(backend)

	// Test rate limiting
	userID := "user:fixed_window:456"
	config := fixedwindow.Config{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 10,
		},
		Window: 10 * time.Second, // Short window for demo
	}

	for i := range 15 {
		result, err := strategy.AllowWithResult(context.Background(), config)
		if err != nil {
			return err
		}

		if result.Allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
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
	redisConfig := redis.RedisConfig{
		Addr:     redisAddr,
		Password: "", // Assuming no password for local Redis
		DB:       0,  // Default DB
		PoolSize: 10,
	}

	backend, err := backends.Create("redis", redisConfig)
	if err != nil {
		// If Redis is not available, skip this example
		fmt.Println("Redis not available, skipping example")
		return nil
	}

	defer func() {
		if err := backend.Close(); err != nil {
			fmt.Printf("Error closing Redis backend: %v\n", err)
		}
	}()

	// Create token bucket strategy
	strategy := tokenbucket.New(backend)

	// Test rate limiting
	userID := "user:token_bucket:redis:789"
	config := tokenbucket.Config{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 15,
		},
		BurstSize:  15,
		RefillRate: 3.0, // 3 tokens per second
	}

	for i := range 20 {
		// Reset the bucket for each request to avoid conflicts with previous runs
		if i == 0 {
			err := strategy.Reset(context.Background(), config)
			if err != nil {
				fmt.Printf("Error in Reset: %v\n", err)
				// Continue with the example even if there's an error
				continue
			}
		}

		result, err := strategy.AllowWithResult(context.Background(), config)
		if err != nil {
			fmt.Printf("Error in AllowWithResult: %v\n", err)
			// Continue with the example even if there's an error
			fmt.Printf("Request %d: ERROR\n", i+1)
			continue
		}

		if result.Allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func tokenBucketPostgresExample() error {
	// Get PostgreSQL connection string from environment variable
	pgConnString := os.Getenv("POSTGRES_CONN_STRING")
	if pgConnString == "" {
		pgConnString = "postgres://postgres:postgres@localhost:5432/ratelimit?sslmode=disable" // Default PostgreSQL connection
	}

	// Create PostgreSQL backend
	pgConfig := postgres.PostgresConfig{
		ConnString: pgConnString,
		MaxConns:   10,
		MinConns:   2,
	}

	backend, err := backends.Create("postgres", pgConfig)
	if err != nil {
		// If PostgreSQL is not available, skip this example
		fmt.Println("PostgreSQL not available, skipping example")
		return nil
	}

	defer func() {
		if err := backend.Close(); err != nil {
			fmt.Printf("Error closing PostgreSQL backend: %v\n", err)
		}
	}()

	// Create token bucket strategy
	strategy := tokenbucket.New(backend)

	// Test rate limiting
	userID := "user:token_bucket:postgres:101"
	config := tokenbucket.Config{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 12,
		},
		BurstSize:  12,
		RefillRate: 2.5, // 2.5 tokens per second
	}

	for i := range 18 {
		// Reset the bucket for each request to avoid conflicts with previous runs
		if i == 0 {
			err := strategy.Reset(context.Background(), config)
			if err != nil {
				fmt.Printf("Error in Reset: %v\n", err)
				// Continue with the example even if there's an error
				continue
			}
		}

		result, err := strategy.AllowWithResult(context.Background(), config)
		if err != nil {
			fmt.Printf("Error in AllowWithResult: %v\n", err)
			// Continue with the example even if there's an error
			fmt.Printf("Request %d: ERROR\n", i+1)
			continue
		}

		if result.Allowed {
			fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
		} else {
			fmt.Printf("Request %d: BLOCKED\n", i+1)
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func statusExample() error {
	// Create memory backend
	backend, _ := backends.Create("memory", nil)

	// Create token bucket strategy
	strategy := tokenbucket.New(backend)

	userID := "user:status:789"
	config := tokenbucket.Config{
		RateLimitConfig: strategies.RateLimitConfig{
			Key:   userID,
			Limit: 10,
		},
		BurstSize:  10,
		RefillRate: 2.0, // 2 tokens per second
	}

	// Reset the bucket to start fresh
	err := strategy.Reset(context.Background(), config)
	if err != nil {
		return err
	}

	// Make some requests to consume tokens
	for i := range 5 {
		result, err := strategy.AllowWithResult(context.Background(), config)
		if err != nil {
			return err
		}

		fmt.Printf("Made request %d: allowed=%v, remaining=%d\n", i+1, result.Allowed, result.Remaining)
		time.Sleep(200 * time.Millisecond)
	}

	// Get current status
	result, err := strategy.GetResult(context.Background(), config)
	if err != nil {
		return err
	}

	fmt.Printf("\nCurrent Status:\n")
	fmt.Printf("  Allowed: %v\n", result.Allowed)
	fmt.Printf("  Remaining: %d\n", result.Remaining)
	fmt.Printf("  Reset Time: %v\n", result.Reset)
	return nil
}

func dualStrategyExample() error {
	// Create a dual strategy rate limiter:
	// - Primary: Fixed Window (hard limits: 10 per minute, 50 per hour)
	// - Secondary: Token Bucket (smoother: 5 burst, 0.5 req/sec refill)
	limiter, err := ratelimit.New(
		ratelimit.WithBackend(memory.New()),
		ratelimit.WithFixedWindowStrategy( // Primary: Hard limits
			ratelimit.TierConfig{Interval: time.Minute, Limit: 10}, // 10 requests per minute
			ratelimit.TierConfig{Interval: time.Hour, Limit: 50},   // 50 requests per hour
		),
		ratelimit.WithTokenBucketStrategy(5, 0.5), // Secondary: 5 burst, 0.5 req/sec refill
		ratelimit.WithBaseKey("api:user:dual"),
	)
	if err != nil {
		return err
	}
	defer limiter.Close()

	fmt.Println("Dual Strategy Rate Limiter:")
	fmt.Println("  Primary (Fixed Window): 10 req/min, 50 req/hour (hard limits)")
	fmt.Println("  Secondary (Token Bucket): 5 burst capacity, 0.5 req/sec refill (smoother)")
	fmt.Println("\nTesting burst behavior...")

	// Test 1: Burst handling - should allow up to 5 requests quickly (smoother limit)
	fmt.Println("\n=== Burst Test (5 quick requests) ===")
	for i := range 5 {
		var results map[string]ratelimit.TierResult
		allowed, err := limiter.Allow(
			ratelimit.WithResult(&results),
			ratelimit.WithContext(context.Background()),
		)
		if err != nil {
			return err
		}

		fmt.Printf("Request %d: %v\n", i+1, map[bool]string{true: "ALLOWED", false: "BLOCKED"}[allowed])

		// Show detailed results
		for strategyName, result := range results {
			if strategyName == "smoother" {
				fmt.Printf("  %s: %d/%d remaining (burst control)\n", strategyName, result.Remaining, result.Total)
			} else {
				fmt.Printf("  %s: %d/%d remaining (hard limit)\n", strategyName, result.Remaining, result.Total)
			}
		}
	}

	// Test 2: Try to exceed burst capacity - should be blocked by smoother
	fmt.Println("\n=== Burst Capacity Test (6th quick request) ===")
	var results map[string]ratelimit.TierResult
	allowed, err := limiter.Allow(
		ratelimit.WithResult(&results),
		ratelimit.WithContext(context.Background()),
	)
	if err != nil {
		return err
	}

	fmt.Printf("Request 6: %v\n", map[bool]string{true: "ALLOWED", false: "BLOCKED"}[allowed])
	for strategyName, result := range results {
		if strategyName == "smoother" {
			fmt.Printf("  %s: %d/%d remaining (burst control - BLOCKED)\n", strategyName, result.Remaining, result.Total)
		} else {
			fmt.Printf("  %s: %d/%d remaining (hard limit)\n", strategyName, result.Remaining, result.Total)
		}
	}

	// Test 3: Wait for token refill and test gradual recovery
	fmt.Println("\n=== Recovery Test (waiting for token refill) ===")
	for i := range 4 {
		time.Sleep(2 * time.Second) // Wait for token bucket to refill

		var results map[string]ratelimit.TierResult
		allowed, err := limiter.Allow(
			ratelimit.WithResult(&results),
			ratelimit.WithContext(context.Background()),
		)
		if err != nil {
			return err
		}

		fmt.Printf("Request %d (after 2s wait): %v\n", i+7, map[bool]string{true: "ALLOWED", false: "BLOCKED"}[allowed])
		for strategyName, result := range results {
			if strategyName == "smoother" {
				fmt.Printf("  %s: %d/%d remaining (burst control)\n", strategyName, result.Remaining, result.Total)
			}
		}
	}

	// Test 4: Try to exceed minute limit - should be blocked by primary fixed window
	fmt.Println("\n=== Minute Limit Test (exceeding 10 req/min) ===")

	// Make 5 more requests quickly (we've already made 9 allowed requests total)
	for i := range 5 {
		var results map[string]ratelimit.TierResult
		allowed, err := limiter.Allow(
			ratelimit.WithResult(&results),
			ratelimit.WithContext(context.Background()),
		)
		if err != nil {
			return err
		}

		fmt.Printf("Request %d: %v\n", i+11, map[bool]string{true: "ALLOWED", false: "BLOCKED"}[allowed])

		// Show primary tier status when blocked
		if !allowed {
			for strategyName, result := range results {
				if strategyName != "smoother" {
					fmt.Printf("  %s: %d/%d remaining (hard limit - BLOCKED)\n", strategyName, result.Remaining, result.Total)
				}
			}
		}
	}

	fmt.Println("\nDual Strategy Summary:")
	fmt.Println("- Primary Fixed Window enforces hard caps (never exceeded)")
	fmt.Println("- Secondary Token Bucket smooths bursts and controls request patterns")
	fmt.Println("- Both strategies must allow for a request to be accepted")

	return nil
}
