package ratelimit

import (
	"fmt"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow"
	"github.com/ajiwo/ratelimit/strategies/gcra"
	"github.com/ajiwo/ratelimit/strategies/leakybucket"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket"
)

// benchmarkConfig holds configuration for dual-strategy benchmarks
type benchmarkConfig struct {
	Name            string
	PrimaryQuotas   map[string]fixedwindow.Quota
	SecondaryType   string
	CreateSecondary func() strategies.Config
}

// Create test configurations for different dual-strategy combinations
var benchmarkConfigs = []benchmarkConfig{
	{
		Name: "FixedWindow3Quotas_TokenBucket",
		PrimaryQuotas: map[string]fixedwindow.Quota{
			"minute_quota": {Limit: 100, Window: time.Minute},
			"second_quota": {Limit: 10, Window: time.Second},
			"hour_quota":   {Limit: 500, Window: time.Hour},
		},
		SecondaryType: "tokenbucket",
		CreateSecondary: func() strategies.Config {
			return tokenbucket.Config{
				BurstSize:  50,
				RefillRate: 10.0,
			}.WithRole(strategies.RoleSecondary)
		},
	},
	{
		Name: "FixedWindow3Quotas_LeakyBucket",
		PrimaryQuotas: map[string]fixedwindow.Quota{
			"minute_quota": {Limit: 100, Window: time.Minute},
			"second_quota": {Limit: 10, Window: time.Second},
			"hour_quota":   {Limit: 500, Window: time.Hour},
		},
		SecondaryType: "leakybucket",
		CreateSecondary: func() strategies.Config {
			return leakybucket.Config{
				Capacity: 50,
				LeakRate: 10.0,
			}.WithRole(strategies.RoleSecondary)
		},
	},
	{
		Name: "FixedWindow3Quotas_GCRA",
		PrimaryQuotas: map[string]fixedwindow.Quota{
			"minute_quota": {Limit: 100, Window: time.Minute},
			"second_quota": {Limit: 10, Window: time.Second},
			"hour_quota":   {Limit: 500, Window: time.Hour},
		},
		SecondaryType: "gcra",
		CreateSecondary: func() strategies.Config {
			return gcra.Config{
				Burst: 50,
				Rate:  10.0,
			}.WithRole(strategies.RoleSecondary)
		},
	},
}

// BenchmarkDualStrategy_Sequential benchmarks dual-strategy in sequential mode
func BenchmarkDualStrategy_Sequential(b *testing.B) {
	for _, config := range benchmarkConfigs {
		b.Run(config.Name, func(b *testing.B) {
			// Create backend
			backend := memory.New()

			// Create primary config (FixedWindow with 3 quotas)
			primaryConfig := fixedwindow.NewConfig().
				SetKey("test").
				AddQuota("minute_quota", 100, time.Minute).
				AddQuota("second_quota", 10, time.Second).
				AddQuota("hour_quota", 500, time.Hour).
				Build()

			// Create secondary config
			secondaryConfig := config.CreateSecondary()

			// Create rate limiter with dual strategy
			limiter, err := New(
				WithBackend(backend),
				WithBaseKey("benchmark"),
				WithPrimaryStrategy(primaryConfig),
				WithSecondaryStrategy(secondaryConfig),
			)
			if err != nil {
				b.Fatalf("Failed to create rate limiter: %v", err)
			}
			defer limiter.Close()

			ctx := b.Context()

			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				allowed, err := limiter.Allow(ctx, AccessOptions{
					Key: fmt.Sprintf("sequential_%d", i%100), // Use different keys to avoid hitting limits
				})
				if err != nil {
					b.Fatalf("Allow failed: %v", err)
				}

				_ = allowed // Use the result
			}
		})
	}
}

// BenchmarkDualStrategy_Concurrent benchmarks dual-strategy in concurrent mode
func BenchmarkDualStrategy_Concurrent(b *testing.B) {
	for _, config := range benchmarkConfigs {
		b.Run(config.Name, func(b *testing.B) {
			// Create backend
			backend := memory.New()

			// Create primary config (FixedWindow with 3 quotas)
			primaryConfig := fixedwindow.NewConfig().
				SetKey("test").
				AddQuota("minute_quota", 100, time.Minute).
				AddQuota("second_quota", 10, time.Second).
				AddQuota("hour_quota", 500, time.Hour).
				Build()

			// Create secondary config
			secondaryConfig := config.CreateSecondary()

			// Create rate limiter with dual strategy
			limiter, err := New(
				WithBackend(backend),
				WithBaseKey("benchmark"),
				WithPrimaryStrategy(primaryConfig),
				WithSecondaryStrategy(secondaryConfig),
			)
			if err != nil {
				b.Fatalf("Failed to create rate limiter: %v", err)
			}
			defer limiter.Close()

			ctx := b.Context()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				requestNum := 0
				for pb.Next() {
					allowed, err := limiter.Allow(ctx, AccessOptions{
						Key: fmt.Sprintf("concurrent_%d", requestNum%100),
					})
					if err != nil {
						b.Fatalf("Allow failed: %v", err)
					}

					requestNum++

					_ = allowed // Use the result
				}
			})
		})
	}
}

// BenchmarkDualStrategy_PeekOnly benchmarks peeking without consuming quota
func BenchmarkDualStrategy_PeekOnly(b *testing.B) {
	for _, config := range benchmarkConfigs {
		b.Run(config.Name, func(b *testing.B) {
			// Create backend
			backend := memory.New()

			// Create primary config (FixedWindow with 3 quotas)
			primaryConfig := fixedwindow.NewConfig().
				SetKey("test").
				AddQuota("minute_quota", 100, time.Minute).
				AddQuota("second_quota", 10, time.Second).
				AddQuota("hour_quota", 500, time.Hour).
				Build()

			// Create secondary config
			secondaryConfig := config.CreateSecondary()

			// Create rate limiter with dual strategy
			limiter, err := New(
				WithBackend(backend),
				WithBaseKey("benchmark"),
				WithPrimaryStrategy(primaryConfig),
				WithSecondaryStrategy(secondaryConfig),
			)
			if err != nil {
				b.Fatalf("Failed to create rate limiter: %v", err)
			}
			defer limiter.Close()

			ctx := b.Context()

			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				allowed, err := limiter.Peek(ctx, AccessOptions{
					Key: fmt.Sprintf("peek_%d", i%100),
				})
				if err != nil {
					b.Fatalf("Peek failed: %v", err)
				}

				_ = allowed // Use the result
			}
		})
	}
}

// BenchmarkDualStrategy_WithResults benchmarks dual-strategy with detailed results
func BenchmarkDualStrategy_WithResults(b *testing.B) {
	for _, config := range benchmarkConfigs {
		b.Run(config.Name, func(b *testing.B) {
			// Create backend
			backend := memory.New()

			// Create primary config (FixedWindow with 3 quotas)
			primaryConfig := fixedwindow.NewConfig().
				SetKey("test").
				AddQuota("minute_quota", 100, time.Minute).
				AddQuota("second_quota", 10, time.Second).
				AddQuota("hour_quota", 500, time.Hour).
				Build()

			// Create secondary config
			secondaryConfig := config.CreateSecondary()

			// Create rate limiter with dual strategy
			limiter, err := New(
				WithBackend(backend),
				WithBaseKey("benchmark"),
				WithPrimaryStrategy(primaryConfig),
				WithSecondaryStrategy(secondaryConfig),
			)
			if err != nil {
				b.Fatalf("Failed to create rate limiter: %v", err)
			}
			defer limiter.Close()

			ctx := b.Context()

			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				var results map[string]strategies.Result
				allowed, err := limiter.Allow(ctx, AccessOptions{
					Key:    fmt.Sprintf("with_results_%d", i%100),
					Result: &results,
				})
				if err != nil {
					b.Fatalf("Allow failed: %v", err)
				}

				// Process results if available
				for key, result := range results {
					_ = key
					_ = result
				}

				_ = allowed // Use the result
			}
		})
	}
}
