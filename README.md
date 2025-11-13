# ratelimit

A Go rate-limiting library with pluggable storage backends and multiple algorithms. 

- Storage backends: in-memory, Redis, Postgres
- Algorithms ("strategies"): Fixed Window (multi-quota), Token Bucket, Leaky Bucket, GCRA
- Dual strategy mode: combine a primary hard limiter with a secondary smoother
- Simple API: Allow, Peek, Reset, Close


## Installation

```bash
go get github.com/ajiwo/ratelimit
```

## Quick start

### Single strategy (Fixed Window, in-memory)

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/ajiwo/ratelimit"
    "github.com/ajiwo/ratelimit/backends/memory"
    "github.com/ajiwo/ratelimit/strategies"
    "github.com/ajiwo/ratelimit/strategies/fixedwindow"
)

func main() {
    // Backend
    mem := memory.New()

    // Allow 5 requests per 3 seconds per user
    limiter, err := ratelimit.New(
        ratelimit.WithBackend(mem),
        ratelimit.WithBaseKey("api"),
        ratelimit.WithPrimaryStrategy(
            fixedwindow.NewConfig().
                AddQuota("default", 5, 3*time.Second).
                Build(),
        ),
    )
    if err != nil { log.Fatal(err) }
    defer limiter.Close()

    ctx := context.Background()
    userID := "user123"

    for i := 1; i <= 7; i++ {
        var results strategies.Results
        allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{Key: userID, Result: &results})
        if err != nil { log.Fatal(err) }
        res := results["default"]
        fmt.Printf("req %d => allowed=%v remaining=%d reset=%s\n", i, allowed, res.Remaining, res.Reset.Format(time.RFC3339))
    }
}
```

### Dual strategy (Composite: Fixed Window + Token Bucket)

Use a strict primary limiter and a burst-smoothing secondary limiter. Both must allow for the request to pass.

```go
limiter, err := ratelimit.New(
    ratelimit.WithBackend(memory.New()),
    ratelimit.WithBaseKey("api"),
    // Primary: hard limits
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("user").
            AddQuota("default", 100, time.Hour).
            Build(),
    ),
    // Secondary: smoothing/burst window (must support CapSecondary)
    ratelimit.WithSecondaryStrategy(tokenbucket.Config{
        BurstSize:  10,   // max burst tokens
        RefillRate: 1.0,  // tokens per second
    }),
)
```

When using dual strategy, the per-quota names in results are prefixed by `primary_` and `secondary_` respectively (e.g., `primary_default`, `secondary_default`).


## Concepts

- Base key: global prefix applied to all rate-limiting keys (e.g., `api:`)
- Dynamic key: runtime dimension like user ID, client IP, or API key
- Strategy config: algorithm-specific configuration implementing `strategies.Config`
- Results: per-quota `strategies.Results` entries with `Allowed`, `Remaining`, `Reset`


## API overview

- `New(opts ...Option) (*RateLimiter, error)`
  - Options: `WithBackend(backends.Backend)`, `WithPrimaryStrategy(strategies.Config)`, `WithSecondaryStrategy(strategies.Config)`, `WithBaseKey(string)`, `WithMaxRetries(int)`
- `(*RateLimiter) Allow(ctx, AccessOptions) (bool, error)`
  - Consumes quota. If `AccessOptions.Result` is provided, receives `strategies.Results`.
- `(*RateLimiter) Peek(ctx, AccessOptions) (bool, error)`
  - Read the current rate limit state without consuming quota; also populates results when provided.
- `(*RateLimiter) Reset(ctx, AccessOptions) error`
  - Resets counters; mainly for testing.
- `(*RateLimiter) Close() error`
  - Releases backend resources, does nothing if backend has been closed.

`AccessOptions`:

```go
type AccessOptions struct {
    Key            string                        // dynamic-key (e.g., user ID)
    SkipValidation bool                          // skip dynamic-key validation if true
    Result         *strategies.Results           // optional results pointer
}
```


## Key validation

Two kinds of keys exist:
- Base key: provided via `WithBaseKey(...)` during construction. This key is always validated and cannot be skipped.
- Dynamic key: provided at access time via `AccessOptions.Key`. This key is validated by default but can be bypassed per-call with `SkipValidation: true`.

Validation rules (applied to both base and dynamic keys unless explicitly skipped for dynamic keys):
- Non-empty and at most 64 bytes
- Allowed characters: ASCII alphanumeric, underscore (_), hyphen (-), colon (:), period (.), and at (@)


## Strategies

Available strategy IDs and capabilities:

- fixed_window
  - Capabilities: Primary, Quotas (multi-quota)
  - Builder: `fixedwindow.NewConfig().SetKey(k).AddQuota(name, limit, window).Build()`
- token_bucket
  - Capabilities: Primary, Secondary
  - Config: `tokenbucket.Config{BurstSize: int, RefillRate: float64}`
- leaky_bucket
  - Capabilities: Primary, Secondary
  - Config: `leakybucket.Config{Capacity: int, LeakRate: float64}`
- gcra
  - Capabilities: Primary, Secondary
  - Config: `gcra.Config{Rate: float64, Burst: int}`

Notes:
- Only Fixed Window supports multiple named quotas simultaneously. See [additional multi-quota documentation](strategies/fixedwindow/MULTI_QUOTA.md).
- When setting a secondary strategy via `WithSecondaryStrategy`, it must advertise `CapSecondary`.
- If a secondary strategy is specified, the primary strategy must not itself be a `CapSecondary`-only secondary in this composite context; the library validates incompatible combinations.


## Backends

Backends implement Get/Set/CheckAndSet/Delete operations. Available implementations:

- In-memory: `github.com/ajiwo/ratelimit/backends/memory`
- Redis: `github.com/ajiwo/ratelimit/backends/redis`
- Postgres: `github.com/ajiwo/ratelimit/backends/postgres`

Use them with `ratelimit.WithBackend(...)`. Example (memory):

```go
limiter, err := ratelimit.New(
    ratelimit.WithBackend(memory.New()),
    ratelimit.WithBaseKey("api"),
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            SetKey("user").
            AddQuota("minute", 10, time.Minute).    // 10 requests per minute
            AddQuota("hour", 100, time.Hour).       // 100 requests per hour
            Build(),
    ),
)
```

## Direct usage with strategies and backends (no ratelimit wrapper)

You can use a strategy directly with a backend if you want full control. In this mode, you construct the storage, the strategy, and the strategy config yourself. The config's Key should represent your full logical key (e.g., base segments + dynamic identifier). Quotas are named and will be returned in the results map.

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/ajiwo/ratelimit/backends/memory"
    fw "github.com/ajiwo/ratelimit/strategies/fixedwindow"
)

func main() {
    ctx := context.Background()

    // 1) Choose a backend
    store := memory.New()

    // 2) Create a strategy bound to the backend
    strat := fw.New(store)

    // 3) Build a strategy config
    // Compose the full key yourself (e.g., base segments + dynamic ID)
    userID := "user123"
    key := "api:user:" + userID
    cfg := fw.NewConfig().
        SetKey(key).
        AddQuota("default", 5, 3*time.Second).
        Build()

    // 4) Perform checks
    // Allow consumes quota and returns per-quota results
    results, err := strat.Allow(ctx, cfg)
    if err != nil {
        panic(err)
    }
    r := results["default"]
    fmt.Printf("allowed=%v remaining=%d reset=%s\n", r.Allowed, r.Remaining, r.Reset.Format(time.RFC3339))

    // Peek inspects current state without consuming quota
    peek, err := strat.Peek(ctx, cfg)
    if err != nil {
        panic(err)
    }
    pr := peek["default"]
    fmt.Printf("peek remaining=%d reset=%s\n", pr.Remaining, pr.Reset.Format(time.RFC3339))
}
```

Notes:
- When using strategies directly, you are responsible for constructing the key string. Follow the same key validation rules as elsewhere.
- Fixed Window supports multiple quotas. Other strategies (Token Bucket, Leaky Bucket, GCRA) have their own configs and typically a single logical limit.


## Middleware examples

The repository includes runnable examples for Echo and net/http.

- Echo: `examples/middleware/echo`
- net/http stdlib: `examples/middleware/stdlib`

Which demonstrate:
- calling `Allow` to enforce
- on 429, calling `Peek` to populate standard headers like `X-RateLimit-Remaining` and `Retry-After`


## Examples directory

The `examples` directory is a Go submodule. Available examples:

- `examples/basic`: single-strategy fixedwindow limiting
- `examples/dual`: dual-strategy fixedwindow + tokenbucket
- `examples/middleware/echo`, `examples/middleware/stdlib`: ready-to-run middleware
- `examples/custom`: shows custom composition/decision logic

Run an example from the `examples` directory:

```bash
cd examples
go run ./basic
```


## Testing

Run the full test suite (root + strategies/backends/tests):

```bash
./test.sh
```

## License

MIT — see [LICENSE](./LICENSE).
