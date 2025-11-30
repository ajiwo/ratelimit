package ratelimit

import (
	"context"
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

// dualStrategyConfig holds configuration for dual-strategy benchmarks
type dualStrategyConfig struct {
	Name            string
	PrimaryQuotas   map[string]fixedwindow.Quota
	SecondaryType   string
	SecondaryConfig strategies.Config
}

// Create primary config (FixedWindow with 3 quotas)
var primaryConfig = fixedwindow.NewConfig().
	AddQuota("minute_quota", 100, time.Minute).
	AddQuota("second_quota", 10, time.Second).
	AddQuota("hour_quota", 500, time.Hour).
	Build()

// Create test configurations for different dual-strategy combinations
var dualStrategyConfigs = []dualStrategyConfig{
	{
		Name:          "FixedWindow3Quotas_TokenBucket",
		SecondaryType: "tokenbucket",
		SecondaryConfig: tokenbucket.Config{
			BurstSize:  50,
			RefillRate: 10.0,
		}.WithRole(strategies.RoleSecondary),
	},
	{
		Name:          "FixedWindow3Quotas_LeakyBucket",
		SecondaryType: "leakybucket",
		SecondaryConfig: leakybucket.Config{
			Capacity: 50,
			LeakRate: 10.0,
		}.WithRole(strategies.RoleSecondary),
	},
	{
		Name:          "FixedWindow3Quotas_GCRA",
		SecondaryType: "gcra",
		SecondaryConfig: gcra.Config{
			Burst: 50,
			Rate:  10.0,
		}.WithRole(strategies.RoleSecondary),
	},
}

type benchmarkMethod func(ctx context.Context, opts AccessOptions) (bool, error)

func createLimiter(b *testing.B, config dualStrategyConfig, index int, method, mode string) *RateLimiter {
	backend := memory.New()
	baseKey := fmt.Sprintf("bench-%s-%s-%d", method, mode, index)
	limiter, err := New(
		WithBackend(backend),
		WithBaseKey(baseKey),
		WithPrimaryStrategy(primaryConfig),
		WithSecondaryStrategy(config.SecondaryConfig),
	)
	if err != nil {
		b.Fatalf("Failed to create rate limiter: %v", err)
	}
	return limiter
}

func runBenchmark(b *testing.B, method benchmarkMethod, concurrent bool) {
	ctx := b.Context()
	b.ResetTimer()
	if concurrent {
		b.RunParallel(func(pb *testing.PB) {
			requestNum := 0
			for pb.Next() {
				var results strategies.Results
				allowed, err := method(ctx, AccessOptions{
					Key:    fmt.Sprintf("concurrent_%d", requestNum%100),
					Result: &results,
				})
				if err != nil {
					b.Fatalf("Method failed: %v", err)
				}
				requestNum++
				for key, result := range results {
					_ = key
					_ = result
				}
				_ = allowed
			}
		})
	} else {
		for i := 0; b.Loop(); i++ {
			var results strategies.Results
			allowed, err := method(ctx, AccessOptions{
				Key:    fmt.Sprintf("with_results_%d", i%100),
				Result: &results,
			})
			if err != nil {
				b.Fatalf("Method failed: %v", err)
			}
			for key, result := range results {
				_ = key
				_ = result
			}
			_ = allowed
		}
	}
}

func BenchmarkDualStrategyAllow_Sequential(b *testing.B) {
	for index, config := range dualStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createLimiter(b, config, index, "allow", "seq")
			defer limiter.Close()
			runBenchmark(b, limiter.Allow, false)
		})
	}
}

func BenchmarkDualStrategyAllow_Concurrent(b *testing.B) {
	for index, config := range dualStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createLimiter(b, config, index, "allow", "par")
			defer limiter.Close()
			runBenchmark(b, limiter.Allow, true)
		})
	}
}

func BenchmarkDualStrategyPeek_Sequential(b *testing.B) {
	for index, config := range dualStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createLimiter(b, config, index, "peek", "seq")
			defer limiter.Close()
			runBenchmark(b, limiter.Peek, false)
		})
	}
}

func BenchmarkDualStrategyPeek_Concurrent(b *testing.B) {
	for index, config := range dualStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createLimiter(b, config, index, "peek", "par")
			defer limiter.Close()
			runBenchmark(b, limiter.Peek, true)
		})
	}
}

// singleStrategyConfig holds configuration for single-strategy benchmarks
type singleStrategyConfig struct {
	Name   string
	Config strategies.Config
}

// Create single strategy configurations
var singleStrategyConfigs = []singleStrategyConfig{
	{
		Name: "FixedWindow",
		Config: fixedwindow.NewConfig().
			AddQuota("default", 100, time.Minute).
			Build(),
	},
	{
		Name: "TokenBucket",
		Config: tokenbucket.Config{
			BurstSize:  100,
			RefillRate: 10.0,
		},
	},
	{
		Name: "LeakyBucket",
		Config: leakybucket.Config{
			Capacity: 100,
			LeakRate: 10.0,
		},
	},
	{
		Name: "GCRA",
		Config: gcra.Config{
			Burst: 100,
			Rate:  10.0,
		},
	},
}

func createSingleLimiter(b *testing.B, config singleStrategyConfig, index int, method string) *RateLimiter {
	backend := memory.New()
	baseKey := fmt.Sprintf("single-bench-%s-%d", method, index)
	limiter, err := New(
		WithBackend(backend),
		WithBaseKey(baseKey),
		WithPrimaryStrategy(config.Config),
	)
	if err != nil {
		b.Fatalf("Failed to create rate limiter: %v", err)
	}
	return limiter
}

func BenchmarkSingleStrategyAllow_Sequential(b *testing.B) {
	for index, config := range singleStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createSingleLimiter(b, config, index, "allow")
			defer limiter.Close()
			runBenchmark(b, limiter.Allow, false)
		})
	}
}

func BenchmarkSingleStrategyAllow_Concurrent(b *testing.B) {
	for index, config := range singleStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createSingleLimiter(b, config, index, "allow")
			defer limiter.Close()
			runBenchmark(b, limiter.Allow, true)
		})
	}
}

func BenchmarkSingleStrategyPeek_Sequential(b *testing.B) {
	for index, config := range singleStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createSingleLimiter(b, config, index, "peek")
			defer limiter.Close()
			runBenchmark(b, limiter.Peek, false)
		})
	}
}

func BenchmarkSingleStrategyPeek_Concurrent(b *testing.B) {
	for index, config := range singleStrategyConfigs {
		b.Run(config.Name, func(b *testing.B) {
			limiter := createSingleLimiter(b, config, index, "peek")
			defer limiter.Close()
			runBenchmark(b, limiter.Peek, true)
		})
	}
}
