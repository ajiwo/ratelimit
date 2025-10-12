# Rate Limiting Library

This is a Go library that implements rate limiting functionality with multiple algorithms and storage backends, including support for single and dual strategy configurations.

## Features

- **Multiple rate limiting algorithms:**
  - Fixed Window
  - Token Bucket
  - Leaky Bucket
- **Multiple storage backends:**
  - In-Memory (single instance)
  - Redis (distributed)
  - PostgreSQL (distributed)
- **Advanced capabilities:**
  - Single strategy rate limiting
  - Dual strategy support (primary + secondary smoother)
  - Multi-tier rate limiting (fixed window with multiple tiers)
  - Detailed statistics and result tracking
  - Dynamic key support for multi-tenant scenarios

## Installation

```bash
go get github.com/ajiwo/ratelimit
```

## Usage

### Single Strategy Rate Limiting

The library provides a high-level rate limiting interface that allows you to configure individual rate limiting strategies:

```go
import (
    "github.com/ajiwo/ratelimit"
    "github.com/ajiwo/ratelimit/backends/memory"
    "github.com/ajiwo/ratelimit/strategies"
)

// Create a backend instance
mem := memory.New()

// Create a rate limiter using functional options
limiter, err := ratelimit.New(
    ratelimit.WithBackend(mem),
    ratelimit.WithPrimaryStrategy(
        strategies.NewFixedWindowConfig("user:123").
            AddTier("default", 100, time.Hour).
            Build(),
    ),
    ratelimit.WithBaseKey("api"),
)
if err != nil {
    panic(err)
}
defer limiter.Close()

// Check if request is allowed (simple version)
allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
if err != nil {
    // handle error
}

// Or get detailed results
var results map[string]strategies.Result
allowed, err := limiter.Allow(
    ratelimit.WithContext(ctx),
    ratelimit.WithResult(&results),
)
if err != nil {
    // handle error
}

// Results include detailed information for each strategy
for strategy, result := range results {
    fmt.Printf("Strategy %s: allowed=%v, remaining=%d, reset=%v\n",
        strategy, result.Allowed, result.Remaining, result.Reset)
}
```

### Multi-Tier Fixed Window Rate Limiting

The fixed window strategy now supports multi-tier configurations, allowing you to define multiple rate limits that all must be satisfied for a request to be accepted:

```go
import (
    "github.com/ajiwo/ratelimit"
    "github.com/ajiwo/ratelimit/backends/memory"
    "github.com/ajiwo/ratelimit/strategies"
)

// Create a backend instance
mem := memory.New()

// Create a multi-tier fixed window rate limiter
limiter, err := ratelimit.New(
    ratelimit.WithBackend(mem),
    ratelimit.WithPrimaryStrategy(strategies.FixedWindowConfig{
        Key: "api:user123",
        Tiers: map[string]strategies.FixedWindowTier{
            "burst": {
                Limit:  10,  // Allow 10 requests per minute
                Window: time.Minute,
            },
            "sustained": {
                Limit:  100, // Allow 100 requests per hour
                Window: time.Hour,
            },
            "daily": {
                Limit:  1000, // Allow 1000 requests per day
                Window: 24 * time.Hour,
            },
        },
    }),
    ratelimit.WithBaseKey("api"),
)
if err != nil {
    panic(err)
}
defer limiter.Close()

// Check if request is allowed (simple version)
allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
if err != nil {
    // handle error
}

// Or get detailed results for all tiers
var results map[string]strategies.Result
allowed, err = limiter.Allow(
    ratelimit.WithContext(ctx),
    ratelimit.WithResult(&results),
)
if err != nil {
    // handle error
}

// Results include detailed information for each tier
for tier, result := range results {
    fmt.Printf("Tier %s: allowed=%v, remaining=%d, reset=%v\n",
        tier, result.Allowed, result.Remaining, result.Reset)
}
```

**Multi-Tier Logic:**
1. **All Tiers Must Allow:** A request is only accepted if all tiers have available quota
2. **Independent Windows:** Each tier operates with its own time window and limit
3. **Atomic Consumption:** When a request is allowed, quota is consumed from all tiers
4. **No Consumption on Denial:** If any tier denies, no quota is consumed from any tier

**Use Cases:**
- **Burst + Sustained:** Allow short bursts while maintaining long-term limits
- **Multiple Time Windows:** Enforce per-minute, per-hour, and per-day limits simultaneously
- **Progressive Limits:** Create graduated rate limiting policies

### Dual Strategy Rate Limiting

The library supports combining a primary strategy with an optional secondary bucket strategy for request smoothing:

```go
import (
    "github.com/ajiwo/ratelimit"
    "github.com/ajiwo/ratelimit/backends/memory"
    "github.com/ajiwo/ratelimit/strategies"
)

// Create a backend instance
mem := memory.New()

// Create a dual-strategy rate limiter
limiter, err := ratelimit.New(
    ratelimit.WithBackend(mem),
    // Primary: strict rate limiting
    ratelimit.WithPrimaryStrategy(
        strategies.NewFixedWindowConfig("api:user").
            AddTier("default", 100, time.Hour).
            Build(),
    ),
    // Secondary: burst smoother (5 burst, 0.1 req/sec refill)
    ratelimit.WithSecondaryStrategy(strategies.TokenBucketConfig{
        BurstSize:  5,
        RefillRate: 0.1,
    }),
    ratelimit.WithBaseKey("api"),
)
if err != nil {
    panic(err)
}
defer limiter.Close()

// Check if request is allowed (simple version)
allowed, err := limiter.Allow(ratelimit.WithContext(ctx))
if err != nil {
    // handle error
}

// Or get detailed results
var results map[string]strategies.Result
allowed, err = limiter.Allow(
    ratelimit.WithContext(ctx),
    ratelimit.WithResult(&results),
)
if err != nil {
    // handle error
}

// Results include both primary and secondary strategy results
for strategy, result := range results {
    fmt.Printf("Strategy %s: allowed=%v, remaining=%d\n", strategy, result.Remaining)
}
```

**Dual Strategy Logic:**
1. **Primary Strategy** provides hard rate limits
2. **Secondary Strategy** acts as a smoother/request shaper
3. **Request Flow:** Check primary first -> If allowed, check secondary smoother
4. **Final Decision:** Both strategies must allow the request

**Supported primary strategies:**
- `strategies.FixedWindowConfig` (supports multi-tier configurations)
- `strategies.TokenBucketConfig`
- `strategies.LeakyBucketConfig`

**Supported secondary strategies (smoothers):**
- `strategies.TokenBucketConfig`
- `strategies.LeakyBucketConfig`

**Multi-tier support:**
- Fixed Window strategy supports multiple tiers with independent limits and windows
- All tiers must allow for a request to be accepted
- Use the builder pattern with `strategies.NewFixedWindowConfig(key).AddTier(name, limit, window).Build()` for both single and multi-tier configurations
- For single-tier configurations, add a single tier named "default"

**Available functional options:**
- `WithBackend(backend)` - Use a custom backend instance
- `WithPrimaryStrategy(strategyConfig)` - Set primary strategy with custom configuration
- `WithSecondaryStrategy(strategyConfig)` - Set secondary smoother strategy
- `WithBaseKey(key)` - Set the base key for rate limiting

**Access Options (for Allow method):**
- `WithContext(ctx)` - Provide context for the operation
- `WithKey(key)` - Use a dynamic key for this specific request
- `WithResult(&results)` - Get detailed results in a map[string]strategies.Result

**Additional Methods:**
- `GetStats(opts...)` - Get detailed statistics for all strategies without consuming quota
- `Reset(opts...)` - Reset rate limit counters (mainly for testing)
- `Close()` - Clean up resources and close the rate limiter

**Strategy Behavior:**
- **Single Strategy:** Use any strategy alone (Fixed Window, Token Bucket, or Leaky Bucket)
- **Dual Strategy:** Combine any primary strategy + bucket-based secondary strategy
- **Strategy Priority:** Primary strategy provides hard limits, secondary strategy provides smoothing

**Limits and Constraints:**
- Arbitrary time intervals are supported (30 seconds, 5 minutes, 2 hours, etc.)
- Secondary strategy must be bucket-based (Token Bucket or Leaky Bucket)
- Primary bucket-based strategy cannot be combined with secondary strategy
- Only one secondary strategy allowed per limiter

**Strategy Configuration Structures:**

```go
// Fixed Window Strategy (Multi-tier Support)
type FixedWindowConfig struct {
    Key   string                             // Unique identifier for the rate limit
    Tiers map[string]FixedWindowTier         // Multiple rate limit tiers
}

type FixedWindowTier struct {
    Limit  int           // Number of requests allowed in the window
    Window time.Duration // Time window (1 minute, 1 hour, 1 day, etc.)
}

// Token Bucket Strategy
type TokenBucketConfig struct {
    Key        string  // Unique identifier for the rate limit
    BurstSize  int     // Maximum tokens the bucket can hold
    RefillRate float64 // Tokens to add per second
}

// Leaky Bucket Strategy
type LeakyBucketConfig struct {
    Key      string  // Unique identifier for the rate limit
    Capacity int     // Maximum requests the bucket can hold
    LeakRate float64 // Requests to process per second
}

// Tier configuration for multi-tier rate limiting
type TierConfig struct {
    Limit  int           // Number of requests allowed in the window
    Window time.Duration // Time window (1 minute, 1 hour, 1 day, etc.)
}

// Helper functions for configuration
func NewFixedWindowConfig(key string) *FixedWindowConfigBuilder  // Returns a builder for creating configurations
func (b *FixedWindowConfigBuilder) AddTier(name string, limit int, window time.Duration) *FixedWindowConfigBuilder  // Add a tier to the configuration
func (b *FixedWindowConfigBuilder) Build() FixedWindowConfig  // Build the final configuration
```

**Result Structure:**
```go
type Result struct {
    Allowed   bool      // Whether the request is allowed
    Remaining int       // Remaining requests in the current window
    Reset     time.Time // When the current window resets
}
```

**Key Validation:**
- Keys must be 1-64 characters long
- Only alphanumeric ASCII, underscore (_), hyphen (-), colon (:), period (.), and at (@) symbols are allowed
- Keys are automatically validated when provided

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


### Advanced Usage

#### Getting Statistics Without Consuming Quota

```go
// Get current statistics without consuming quota
var stats map[string]strategies.Result
stats, err := limiter.GetStats(
    ratelimit.WithContext(ctx),
    ratelimit.WithKey("user:123"),
)
if err != nil {
    // handle error
}

for strategy, result := range stats {
    fmt.Printf("Strategy %s: %d remaining, resets at %v\n",
        strategy, result.Remaining, result.Reset)
}
```

#### Resetting Rate Limits

```go
// Reset rate limits for a specific key (useful for testing)
err := limiter.Reset(
    ratelimit.WithContext(ctx),
    ratelimit.WithKey("user:123"),
)
if err != nil {
    // handle error
}
```

#### Dynamic Key Support

```go
// Use different keys for different users/endpoints
allowed, err := limiter.Allow(
    ratelimit.WithContext(ctx),
    ratelimit.WithKey("user:123"), // Dynamic key
)
```

### Rate Limiting Strategies

For direct strategy usage, you can create strategies individually:

#### Fixed Window

```go
import (
    "github.com/ajiwo/ratelimit/backends/memory"
    "github.com/ajiwo/ratelimit/strategies/fixedwindow"
)

// Create a memory backend instance
storage := memory.New()

// Create a fixed window strategy
strategy := fixedwindow.New(storage)

// Configure rate limiting (single-tier)
config := strategies.NewFixedWindowConfig("user:123").
    AddTier("default", 100, time.Minute).
    Build()

// Or configure multi-tier rate limiting
multiTierConfig := strategies.FixedWindowConfig{
    Key: "user:123",
    Tiers: map[string]strategies.FixedWindowTier{
        "burst": {
            Limit:  10,
            Window: time.Minute,
        },
        "sustained": {
            Limit:  100,
            Window: time.Hour,
        },
    },
}

// Check if request is allowed and get detailed results
results, err := strategy.Allow(ctx, config) // Returns map[string]strategies.Result for multi-tier
if err != nil {
    // Handle error
}

// For single-tier, access the "default" result
result := results["default"]
if result.Allowed {
    fmt.Printf("Request allowed, %d remaining, resets at %v\n",
        result.Remaining, result.Reset)
    // Process request
} else {
    fmt.Printf("Request blocked, %d remaining, resets at %v\n",
        result.Remaining, result.Reset)
    // Reject request
}

// For multi-tier, iterate through all tier results
for tierName, tierResult := range results {
    fmt.Printf("Tier %s: allowed=%v, remaining=%d, reset=%v\n",
        tierName, tierResult.Allowed, tierResult.Remaining, tierResult.Reset)
}
```

#### Token Bucket

```go
// Create a memory backend instance
storage := memory.New()

// Create a token bucket strategy
strategy := tokenbucket.New(storage)

// Configure rate limiting
config := strategies.TokenBucketConfig{
    Key:        "user:123",
    BurstSize:  10,
    RefillRate: 1.0, // 1 token per second
}

// Check if request is allowed and get detailed results
result, err := strategy.Allow(ctx, config)
if err != nil {
    // Handle error
}
if result.Allowed {
    fmt.Printf("Request allowed, %d remaining, resets at %v\n",
        result.Remaining, result.Reset)
    // Process request
} else {
    fmt.Printf("Request blocked, %d remaining, resets at %v\n",
        result.Remaining, result.Reset)
    // Reject request
}
```

#### Leaky Bucket

```go
// Create a memory backend instance
storage := memory.New()

// Create a leaky bucket strategy
strategy := leakybucket.New(storage)

// Configure rate limiting
config := strategies.LeakyBucketConfig{
    Key:      "user:123",
    Capacity: 50,  // Maximum requests the bucket can hold
    LeakRate: 2.0, // 2 requests per second leak rate
}

// Check if request is allowed and get detailed results
result, err := strategy.Allow(ctx, config)
if err != nil {
    // Handle error
}
if result.Allowed {
    fmt.Printf("Request allowed, %d remaining, resets at %v\n",
        result.Remaining, result.Reset)
    // Process request
} else {
    fmt.Printf("Request blocked, %d remaining, resets at %v\n",
        result.Remaining, result.Reset)
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