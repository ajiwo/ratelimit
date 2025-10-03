# Rate Limiting Library

This is a Go library that implements rate limiting functionality with multiple algorithms and storage backends, including high-level multi-tier rate limiting support.

## Features

- Multiple rate limiting algorithms:
  - Fixed Window
  - Token Bucket
  - Leaky Bucket
- Multiple storage backends:
  - In-Memory (single instance)
  - Redis (distributed)
  - PostgreSQL (distributed)

## Installation

```bash
go get github.com/ajiwo/ratelimit
```

## Usage

### Multi-Tier Rate Limiting

The library provides a high-level multi-tier rate limiting interface that allows you to configure multiple time-based rate limits simultaneously:

```go
import (
    "github.com/ajiwo/ratelimit"
    "github.com/ajiwo/ratelimit/backends/memory"
)

// Create a backend instance
mem := memory.New()

// Create a rate limiter using functional options
limiter, err := ratelimit.New(
    ratelimit.WithBackend(mem),
    ratelimit.WithFixedWindowStrategy(
        ratelimit.TierConfig{Interval: time.Minute, Limit: 5},    // 5 requests per minute
        ratelimit.TierConfig{Interval: time.Hour, Limit: 100},   // 100 requests per hour
        ratelimit.TierConfig{Interval: 24 * time.Hour, Limit: 1000}, // 1000 requests per day
    ),
    ratelimit.WithBaseKey("user:123"),
)
if err != nil {
    panic(err)
}
defer limiter.Close()

// Check if request is allowed
allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
```

The multi-tier limiter enforces ALL tiers simultaneously - a request is only allowed if it passes ALL configured rate limit tiers.

### Dual Strategy Rate Limiting

The library supports combining a primary tier-based strategy (like Fixed Window) with an optional secondary bucket strategy (Token Bucket or Leaky Bucket) for request smoothing:

```go
import (
    "github.com/ajiwo/ratelimit"
    "github.com/ajiwo/ratelimit/backends/memory"
)

// Create a backend instance
mem := memory.New()

// Create a dual-strategy rate limiter
limiter, err := ratelimit.New(
    ratelimit.WithBackend(mem),
    ratelimit.WithFixedWindowStrategy(                    // Primary: strict rate limiting
        ratelimit.TierConfig{Interval: time.Minute, Limit: 10},   // 10 requests per minute
        ratelimit.TierConfig{Interval: time.Hour, Limit: 100},    // 100 requests per hour
    ),
    ratelimit.WithTokenBucketStrategy(5, 0.1),             // Secondary: burst smoother (5 burst, 0.1 req/sec refill)
    ratelimit.WithBaseKey("user"),
)
if err != nil {
    panic(err)
}
defer limiter.Close()

// Check if request is allowed
allowed, results, err := limiter.AllowWithResult(ratelimit.WithContext(ctx))
if err != nil {
    // handle error
}

// Results include both tier-based and smoother strategy results
for strategy, result := range results {
    fmt.Printf("Strategy %s: allowed=%v, remaining=%d\n", strategy, result.Allowed, result.Remaining)
}
```

**Dual Strategy Logic:**
1. **Primary Strategy** (Fixed Window, etc.) provides hard rate limits
2. **Secondary Strategy** (Token/Leaky Bucket) acts as a smoother/shaper
3. **Request Flow:** Check primary first → If allowed, check secondary smoother
4. **Final Decision:** Both strategies must allow the request

**Supported primary strategies:**
- `ratelimit.StrategyFixedWindow`

**Supported secondary strategies (smoothers):**
- `ratelimit.StrategyTokenBucket`
- `ratelimit.StrategyLeakyBucket`

**Available functional options:**
- `WithBackend(backend)` - Use a custom backend instance
- `WithFixedWindowStrategy(tiers...)` - Fixed window algorithm (can be primary or standalone)
- `WithTokenBucketStrategy(burstSize, refillRate)` - Token bucket algorithm (can be primary, secondary smoother, or standalone)
- `WithLeakyBucketStrategy(capacity, leakRate)` - Leaky bucket algorithm (can be primary, secondary smoother, or standalone)
- `WithPrimaryStrategy(strategy, tiers...)` - Explicitly set primary strategy
- `WithBaseKey(key)` - Set the base key for rate limiting
- `WithTiers(tiers...)` - Override default tiers for fixed window strategy
- `WithCleanupInterval(interval)` - Set cleanup interval for internal stale data

**Strategy Behavior:**
- **Single Strategy:** Use any strategy alone (Fixed Window, Token Bucket, or Leaky Bucket)
- **Dual Strategy:** Combine Fixed Window (primary) + Token/Leaky Bucket (secondary smoother)
- **Strategy Priority:** First strategy option becomes primary, subsequent bucket options become secondary

**Limits and Constraints:**
- Fixed Window: Maximum 12 tiers per configuration, minimum interval: 5 seconds
- Each tier must have a positive limit
- Arbitrary time intervals are supported (30 seconds, 5 minutes, 2 hours, etc.)
- Bucket strategies (Token Bucket, Leaky Bucket) don't use tiers
- Fixed Window cannot be used as secondary strategy
- Only one secondary strategy allowed per limiter

### Storage Backends

The library supports multiple storage backends:

#### 1. In-Memory Storage

```go
import "github.com/ajiwo/ratelimit/backends/memory"

// Create memory backend instance
mem := memory.New()

limiter, err := ratelimit.New(
    ratelimit.WithBackend(mem),
    // ... other options
)
```

#### 2. Redis Storage

```go
import "github.com/ajiwo/ratelimit/backends/redis"

// Create Redis backend instance
redisBackend, err := redis.New(redis.RedisConfig{
    Addr:     "localhost:6379",
    Password: "", // no password
    DB:       0,  // default DB
    PoolSize: 10,
})
if err != nil {
    // handle error
}

limiter, err := ratelimit.New(
    ratelimit.WithBackend(redisBackend),
    // ... other options
)
```

#### 3. PostgreSQL Storage

```go
import "github.com/ajiwo/ratelimit/backends/postgres"

// Create PostgreSQL backend instance
pgBackend, err := postgres.New(postgres.PostgresConfig{
    ConnString: "postgres://user:pass@localhost/db",
    MaxConns:   10,
    MinConns:   2,
})
if err != nil {
    // handle error
}

limiter, err := ratelimit.New(
    ratelimit.WithBackend(pgBackend),
    // ... other options
)
```


### Rate Limiting Strategies

For direct strategy usage, you can create strategies individually:

#### Fixed Window

```go
import (
    "github.com/ajiwo/ratelimit/backends/memory"
    "github.com/ajiwo/ratelimit/strategies"
)

// Create a memory backend instance
storage := memory.New()

// Create a fixed window strategy
strategy := strategies.NewFixedWindow(storage)

// Configure rate limiting
config := strategies.FixedWindowConfig{
    RateLimitConfig: strategies.RateLimitConfig{
        Key:   "user:123",
        Limit: 100,
    },
    Window: time.Minute, // Time window
}

// Check if request is allowed
allowed, err := strategy.Allow(ctx, config)
if err != nil {
    // Handle error
}
if allowed {
    // Process request
} else {
    // Reject request
}
```

#### Token Bucket

```go
// Create a memory backend instance
storage := memory.New()

// Create a token bucket strategy
strategy := strategies.NewTokenBucket(storage)

// Configure rate limiting
config := strategies.TokenBucketConfig{
    RateLimitConfig: strategies.RateLimitConfig{
        Key:   "user:123",
        Limit: 100,
    },
    BurstSize:  10,
    RefillRate: 1.0, // 1 token per second
}

// Check if request is allowed
allowed, err := strategy.Allow(ctx, config)
if err != nil {
    // Handle error
}
if allowed {
    // Process request
} else {
    // Reject request
}
```

#### Leaky Bucket

```go
// Create a memory backend instance
storage := memory.New()

// Create a leaky bucket strategy
strategy := strategies.NewLeakyBucket(storage)

// Configure rate limiting
config := strategies.LeakyBucketConfig{
    RateLimitConfig: strategies.RateLimitConfig{
        Key:   "user:123",
        Limit: 100,
    },
    Capacity: 50,  // Maximum requests the bucket can hold
    LeakRate: 2.0, // 2 requests per second leak rate
}

// Check if request is allowed
allowed, err := strategy.Allow(ctx, config)
if err != nil {
    // Handle error
}
if allowed {
    // Process request
} else {
    // Reject request
}
```

## Testing

Run tests with:

```bash
./test.sh
```

Note: Some tests require running Redis and PostgreSQL instances for integration testing.

## License

MIT
