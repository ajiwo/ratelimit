# Rate Limiting Library

This is a Go library that implements rate limiting functionality with multiple algorithms and storage backends, including high-level multi-tier rate limiting support.

## Features

- Multiple rate limiting algorithms:
  - Fixed Window
  - Token Bucket
- Multiple storage backends:
  - In-Memory (single instance)
  - Redis (distributed)

## Installation

```bash
go get github.com/ajiwo/ratelimit
```

## Usage

### Multi-Tier Rate Limiting

The library provides a high-level multi-tier rate limiting interface that allows you to configure multiple time-based rate limits simultaneously:

```go
import "github.com/ajiwo/ratelimit"

// Create a rate limiter using functional options
limiter, err := ratelimit.New(
    ratelimit.WithMemoryBackend(),
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
allowed, err := limiter.Allow(ctx)
```

The multi-tier limiter enforces ALL tiers simultaneously - a request is only allowed if it passes ALL configured rate limit tiers.

**Supported strategies:**
- `ratelimit.StrategyFixedWindow`
- `ratelimit.StrategyTokenBucket`

**Available functional options:**
- `WithMemoryBackend()` - Use in-memory storage
- `WithRedisBackend(config)` - Use Redis storage
- `WithFixedWindowStrategy(tiers...)` - Fixed window algorithm
- `WithTokenBucketStrategy(burstSize, refillRate, tiers...)` - Token bucket algorithm
- `WithBaseKey(key)` - Set the base key for rate limiting
- `WithTiers(tiers...)` - Override default tiers for any strategy

**Limits and Constraints:**
- Maximum 12 tiers per configuration
- Minimum interval: 5 seconds
- Each tier must have a positive limit
- Arbitrary time intervals are supported (30 seconds, 5 minutes, 2 hours, etc.)

### Storage Backends

The library supports multiple storage backends:

### 1. In-Memory Storage

```go
storage := backends.NewMemoryStorage()
```

### 2. Redis Storage

```go
config := backends.RedisConfig{
    Addr:     "localhost:6379",
    Password: "",
    DB:       0,
}
storage, err := backends.NewRedisStorage(config)
```



### Rate Limiting Strategies

After creating a storage backend, you can use it with different rate limiting strategies:

#### Fixed Window

```go
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

## Testing

Run tests with:

```bash
./test.sh
```

Note: Some tests require running Redis instances for integration testing.

## License

MIT
