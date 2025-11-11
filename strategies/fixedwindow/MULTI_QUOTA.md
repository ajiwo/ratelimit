# Fixed Window Multi-Quota Usage

The Fixed Window strategy's multi-quota feature allows you to apply **multiple time windows** to the **same operation**. This is commonly used for tiered rate limiting like "10 requests per minute AND 100 requests per hour AND 1000 requests per day".

## Intended Usage: Time-based Quotas

Multi-quota is designed for applying multiple time constraints to the **same type of operation**:

```go
// Correct: Multiple time windows for API calls
limiter, _ := ratelimit.New(
    ratelimit.WithBackend(memory.New()),
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("api_calls").
            AddQuota("minute", 10, time.Minute).    // 10 calls per minute
            AddQuota("hour", 100, time.Hour).       // 100 calls per hour
            AddQuota("day", 1000, 24*time.Hour).    // 1000 calls per day
            Build(),
    ),
)

// Request passes only if ALL quotas have capacity
allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
    Key: "user123",
})

```

**Examples:**
- API rate limiting: 60 requests/minute, 1000 requests/hour, 10000 requests/day
- Login attempts: 5 attempts/minute, 20 attempts/hour, 100 attempts/day
- File uploads: 2 uploads/minute, 50 uploads/hour, 200 uploads/day

## Common Misconception: Different Operations

```go
limiter, _ := ratelimit.New(
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("mixed_operations").
            AddQuota("auth", 5, time.Minute).      // Different operation types
            AddQuota("search", 100, time.Minute).  // should use separate limiters
            AddQuota("upload", 10, time.Hour).     // not multi-quota for same operation
            Build(),
    ),
)

```

## Separate Limiters

Because mixing different operation types in one limiter is not supported, do these approach instead.

**Separate limiters for different operations**

```go
// One limiter per operation type
authLimiter, _ := ratelimit.New(
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("auth").
            AddQuota("minute", 5, time.Minute).
            AddQuota("hour", 50, time.Hour).
            Build(),
    ),
)

searchLimiter, _ := ratelimit.New(
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("search").
            AddQuota("minute", 100, time.Minute).
            AddQuota("hour", 1000, time.Hour).
            Build(),
    ),
)

```

**Separate limiters for different user tiers**

```go
// Standard tier: 100 requests/hour
standardLimiter, _ := ratelimit.New(
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("standard").
            AddQuota("minute", 2, time.Minute).
            AddQuota("hour", 100, time.Hour).
            Build(),
    ),
)

// Premium tier: 1000 requests/hour
premiumLimiter, _ := ratelimit.New(
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("premium").
            AddQuota("minute", 20, time.Minute).
            AddQuota("hour", 1000, time.Hour).
            Build(),
    ),
)

```

## How Multi-Quota Works

- **Atomic Evaluation**: ALL quotas must have capacity for a request to pass
- **Simultaneous Consumption**: When allowed, ALL quotas are incremented
- **Independent Reset**: Each quota resets based on its own time window
- **Failure**: If ANY quota is exceeded, the request is denied

## Response Format

```go
var results strategies.Results
allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
    Key: "user123",
    Result: &results,
})
// results contains:
// {
//     "minute": {Allowed: true, Remaining: 9, Reset: ...},
//     "hour":   {Allowed: true, Remaining: 99, Reset: ...},
//     "day":    {Allowed: true, Remaining: 999, Reset: ...},
// }
// allowed? true (all quotas passed)

```
