package main

import (
	"context"
	"fmt"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
)

type strategyResults = map[string]strategies.Result

// customStrategy allows manual composition of strategies
type customStrategy struct {
	primary   strategies.Strategy
	secondary strategies.Strategy
	storage   backends.Backend
}

// newCustomStrategy creates a new custom composer
func newCustomStrategy(storage backends.Backend,
	primaryName, secondaryName string) *customStrategy {

	composer := &customStrategy{
		storage: storage,
	}

	// Create strategies based on names
	switch primaryName {
	case "fixed_window":
		composer.primary = fixedwindow.New(storage)
	case "token_bucket":
		composer.primary = tokenbucket.New(storage)
	}

	switch secondaryName {
	case "token_bucket":
		composer.secondary = tokenbucket.New(storage)
	}

	return composer
}

// Peek checks both strategies without consuming quota
func (c *customStrategy) Peek(ctx context.Context,
	primaryConfig, secondaryConfig strategies.StrategyConfig) (strategyResults, error) {

	results := make(strategyResults)

	// Check primary strategy without consuming quota
	primaryResults, err := c.primary.GetResult(ctx, primaryConfig)
	if err != nil {
		return nil, fmt.Errorf("primary strategy check failed: %w", err)
	}

	// Add primary results to map
	for key, result := range primaryResults {
		results["primary_"+key] = result
	}

	// Check secondary strategy without consuming quota
	secondaryResults, err := c.secondary.GetResult(ctx, secondaryConfig)
	if err != nil {
		return nil, fmt.Errorf("secondary strategy check failed: %w", err)
	}

	// Add secondary results to map
	for key, result := range secondaryResults {
		results["secondary_"+key] = result
	}

	return results, nil
}

// Allow allows request based on custom decision logic
func (c *customStrategy) Allow(
	ctx context.Context,
	primaryConfig, secondaryConfig strategies.StrategyConfig,
	logic func(strategyResults) bool) (bool, strategyResults, error) {

	results, err := c.Peek(ctx, primaryConfig, secondaryConfig)
	if err != nil {
		return false, nil, err
	}

	// Apply custom logic to determine if request should be allowed
	decision := logic(results)

	if decision {
		// Consume quota from both strategies
		_, err := c.primary.Allow(ctx, primaryConfig)
		if err != nil {
			return false, nil, fmt.Errorf("failed to consume primary quota: %w", err)
		}

		_, err = c.secondary.Allow(ctx, secondaryConfig)
		if err != nil {
			return false, nil, fmt.Errorf("failed to consume secondary quota: %w", err)
		}
	}

	return decision, results, nil
}

func main() {
	// Create storage backend
	storage := memory.New()

	// Create custom composer with fixed window as primary and token bucket as secondary
	composer := newCustomStrategy(storage, "fixed_window", "token_bucket")

	// Configure strategies
	primaryConfig := fixedwindow.NewConfig("user:123").
		AddQuota("hourly", 100, time.Hour).
		Build()

	secondaryConfig := tokenbucket.Config{
		Key:        "user:123",
		BurstSize:  5,
		RefillRate: 0.1, // 1 token per 10 seconds
	}

	ctx := context.Background()

	// Example 1: Check-only mode (no quota consumption)
	fmt.Println("=== Check Only Mode ===")
	results, err := composer.Peek(ctx, primaryConfig, secondaryConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	for name, result := range results {
		fmt.Printf("Strategy %s: allowed=%v, remaining=%d, reset=%v\n",
			name, result.Allowed, result.Remaining, result.Reset)
	}

	// Example 2: Custom AND logic (both must allow)
	fmt.Println("\n=== Custom AND Logic ===")

	andLogic := func(res strategyResults) bool {
		// Both primary and secondary must allow
		primaryAllowed := res["primary_hourly"].Allowed
		secondaryAllowed := res["secondary_default"].Allowed
		return primaryAllowed && secondaryAllowed
	}

	allowed, _ /*results*/, err := composer.Allow(ctx, primaryConfig, secondaryConfig, andLogic)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Request allowed: %v\n", allowed)

	// Example 3: Custom OR logic (either can allow)
	fmt.Println("\n=== Custom OR Logic ===")

	orLogic := func(res strategyResults) bool {
		// At least one must allow
		primaryAllowed := res["primary_hourly"].Allowed
		secondaryAllowed := res["secondary_default"].Allowed
		return primaryAllowed || secondaryAllowed
	}

	allowed, _ /*results*/, err = composer.Allow(ctx, primaryConfig, secondaryConfig, orLogic)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Request allowed: %v\n", allowed)

	// Clean up
	_ = storage.Close()
}
