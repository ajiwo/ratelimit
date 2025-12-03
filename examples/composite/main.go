package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/internal/strategies/composite"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
)

// This example demonstrates using the internal composite strategy directly
// without the ratelimit.RateLimiter wrapper and without building your own
// manual composer. You construct primary and secondary configs, then execute
// the composite strategy with a composite config and dynamic key.
func Example() error {
	ctx := context.Background()

	// 1) Choose a backend
	store := memory.New()
	defer store.Close()

	// 2) Build primary and secondary strategy configs
	// Primary: Fixed Window (hard quota)
	primaryCfg := fixedwindow.NewConfig().
		SetKey("user"). // semantic key segment for primary; composite uses an internal key
		AddQuota("default", 20, 30*time.Second).
		Build()

	// Secondary: Token Bucket (burst smoother)
	secondaryCfg := &tokenbucket.Config{
		Burst: 5,    // up to 5 requests burst
		Rate:  1.25, // ~1.25 tokens per second
	}

	// 3) Create composite strategy bound to backend
	comp, err := composite.New(store, primaryCfg, secondaryCfg)
	if err != nil {
		return fmt.Errorf("failed to create composite strategy: %v", err)
	}

	// 4) Build the composite config with BaseKey and dynamic key
	// BaseKey is the stable project namespace; key is the runtime dimension
	baseKey := "api"
	dynamicKey := "user42" // e.g., user ID, API key, IP

	compCfg := (&composite.Config{
		BaseKey:   baseKey,
		Primary:   primaryCfg,
		Secondary: secondaryCfg,
	}).WithKey(dynamicKey).WithMaxRetries(200)

	fmt.Println("=== Composite Strategy (Direct Usage) ===")
	fmt.Println("Primary: Fixed Window 20 per 30s")
	fmt.Println("Secondary: Token Bucket burst=5 refill=1.25/s")
	fmt.Printf("Key: %s:%s\n\n", baseKey, dynamicKey)

	// 5) Make a few requests within limits
	for i := 1; i <= 6; i++ {
		results, err := comp.Allow(ctx, compCfg)
		if err != nil {
			return fmt.Errorf("allow failed: %v", err)
		}
		allowed := allAllowed(results)

		fmt.Printf("req %d => %s\n", i, allowedStr(allowed))
		printCompositeResults(results)
	}

	// 6) Burst test: send more than burst size quickly
	fmt.Println("\n=== Burst test (exceed secondary burst) ===")
	for i := 1; i <= 8; i++ {
		results, err := comp.Allow(ctx, compCfg)
		if err != nil {
			return fmt.Errorf("allow failed: %v", err)
		}
		allowed := allAllowed(results)
		fmt.Printf("burst req %d => %s\n", i, allowedStr(allowed))
	}

	// 7) Peek without consuming quota
	fmt.Println("\n=== Peek (no consumption) ===")
	peek, err := comp.Peek(ctx, compCfg)
	if err != nil {
		return fmt.Errorf("peek failed: %v", err)
	}
	printCompositeResults(peek)

	// 8) Wait to observe token refill
	fmt.Println("\nSleeping 3s to observe token refill...")
	time.Sleep(3 * time.Second)

	results, err := comp.Allow(ctx, compCfg)
	if err != nil {
		return fmt.Errorf("post-sleep allow failed: %v", err)
	}
	fmt.Println("after 3s =>", allowedStr(allAllowed(results)))
	printCompositeResults(results)
	return nil
}

// allAllowed returns true if all results indicate Allowed
func allAllowed(r strategies.Results) bool {
	for _, v := range r {
		if !v.Allowed {
			return false
		}
	}
	return true
}

func allowedStr(b bool) string {
	if b {
		return "ALLOWED"
	}
	return "DENIED"
}

// printCompositeResults prints primary_*/secondary_* entries succinctly
func printCompositeResults(r strategies.Results) {
	p, s := r.PrimaryDefault(), r.SecondaryDefault()
	fmt.Printf("  primary_default  => allowed=%v remaining=%d reset=%s\n", p.Allowed, p.Remaining, p.Reset.Format(time.RFC3339))
	fmt.Printf("  secondary_default=> allowed=%v remaining=%d reset=%s\n", s.Allowed, s.Remaining, s.Reset.Format(time.RFC3339))
}

func main() {
	if err := Example(); err != nil {
		log.Fatal(err)
	}
}
