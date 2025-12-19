# ratelimit

Go rate limiting library with multiple algorithm and storage options. 

- Storage **backends**: in-memory, Redis, Postgres
- **Algorithms** ("strategies"): Fixed Window (multi-quota), Token Bucket, Leaky Bucket, GCRA
- **Dual strategy** mode: combine a primary hard limiter with a secondary smoother

## Installation

```bash
go get github.com/ajiwo/ratelimit
```

## Usage

Using the library involves these main steps:

1. **Choose a backend**: Select a storage backend (memory, Redis, or Postgres)
2. **Create a limiter**: Configure rate limiting strategies using `ratelimit.New()`
3. **Make access requests**: Call `Allow()` to consume quota or `Peek()` to check status
4. **Handle results**: Check the allowed status and examine quota information
5. **Clean up**: Call `Close()` to release backend resources

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
        allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
            Key: userID,
            Result: &results,
        })
        if err != nil { log.Fatal(err) }

        res := results.Default() 
        fmt.Printf(
            "req %d => allowed=%v remaining=%d reset=%s\n",
            i, allowed, res.Remaining, res.Reset.Format(time.RFC3339),
        )
    }
}
```

### Dual strategy (Fixed Window + Token Bucket)

Use a strict primary limiter and a burst-smoothing secondary limiter. Both must allow for the request to pass.

```go
limiter, err := ratelimit.New(
    ratelimit.WithBackend(memory.New()),
    ratelimit.WithBaseKey("api"),
    // Primary: hard limits
    ratelimit.WithPrimaryStrategy(
        fixedwindow.NewConfig().
            AddQuota("hourly", 100, time.Hour).
            Build(),
    ),
    // Secondary: smoothing/burst window (must support CapSecondary)
    ratelimit.WithSecondaryStrategy(&tokenbucket.Config{
        Burst: 10,     // max burst tokens
        Rate:  1.0,    // tokens per second
    }),
)
```

When using dual strategy, the per-quota names in results are prefixed by `primary_` and `secondary_` respectively (e.g., `primary_hourly`, `secondary_default`).


## Concepts

- Base key: global prefix applied to all rate-limiting keys (e.g., `api:`)
- Dynamic key: runtime dimension like user ID, client IP, or API key
- Strategy config: algorithm-specific configuration implementing `strategies.Config`
- Results: per-quota `strategies.Results` entries with `Allowed`, `Remaining`, `Reset`


## Results helper methods

The `strategies.Results` type provides convenient helper methods for accessing quota results:

```go
// For single quota scenarios
result := results.Default()                   // Get the "default" quota result

// For dual strategy scenarios
primaryResult := results.PrimaryDefault()     // Get "primary_default"
secondaryResult := results.SecondaryDefault() // Get "secondary_default"

// For specific named quotas
result := results.Quota("hourly")             // Get a specific quota by name
primaryHourly := results.Primary("hourly")    // Get "primary_hourly"

// Check if a quota exists
if results.HasQuota("primary_daily") { ... }

// Utility methods
anyAllowed := results.AnyAllowed()            // true if any quota allows the request
allAllowed := results.AllAllowed()            // true if all quotas allow the request
firstResult := results.First()                // get first result (use with caution)
count := results.Len()                        // number of quotas in results
```

## API overview

- `New(opts ...Option) (*Limiter, error)`
  - Options:
    - `WithBackend(backends.Backend)`
    - `WithPrimaryStrategy(strategies.Config)`
    - `WithSecondaryStrategy(strategies.Config)`
    - `WithBaseKey(string)`
    - `WithMaxRetries(int)`
- `(*Limiter) Allow(ctx, AccessOptions) (bool, error)`
  - Consumes quota. If `AccessOptions.Result` is provided, receives `strategies.Results`.
- `(*Limiter) Peek(ctx, AccessOptions) (bool, error)`
  - Read the current rate limit state without consuming quota; also populates results when provided.
- `(*Limiter) Reset(ctx, AccessOptions) error`
  - Resets counters; mainly for testing.
- `(*Limiter) Close() error`
  - Releases backend resources, does nothing if backend has been closed.

`AccessOptions`:

```go
type AccessOptions struct {
    Key            string                     // dynamic-key (e.g., user ID)
    SkipValidation bool                       // skip dynamic-key validation if true
    Result         *strategies.Results        // optional results pointer
}
```


## Key validation

Two kinds of keys exist:
- Base key: provided via `WithBaseKey(...)` during construction. This key is always validated and cannot be skipped.
- Dynamic key: provided at access time via `AccessOptions.Key`. This key is validated by default but can be bypassed per-call with `SkipValidation: true`.

Validation rules (applied to both base and dynamic keys unless explicitly skipped for dynamic keys):
- Non-empty and at most 64 bytes
- Allowed characters: ASCII alphanumeric, underscore (_), hyphen (-), colon (:), period (.), at (@), and plus (+)


## Strategies

Available strategy IDs and capabilities:

- fixed_window
  - Capabilities: Primary, Up to 8 Quotas per key
  - Config:
    ```go
    fixedwindow.NewConfig().
        SetKey(k).                      // ignored when used with limiter
        SetMaxRetries(r).               // optional, unset or 0 uses default, 1 to disable retries
        AddQuota(name, limit, window).
        Build()
    ```
- token_bucket
  - Capabilities: Primary, Secondary
  - Config: 
    ```go
    &tokenbucket.Config{
        Key:        string,             // ignored when used with limiter
        MaxRetries: int,                // unset or 0 uses default, 1 to disable retries
        Burst:      int,                // max burst tokens
        Rate:       float64,            // refill rate (tokens per second)
    }
    ```
- leaky_bucket
  - Capabilities: Primary, Secondary
  - Config: 
    ```go
    &leakybucket.Config{
        Key:        string,
        MaxRetries: int,
        Burst:      int,                // max burst requests
        Rate:       float64,            // leak rate (requests per second)
    }
    ```
- gcra
  - Capabilities: Primary, Secondary
  - Config:
    ```go
    &gcra.Config{
        Key:        string,
        MaxRetries: int,
        Burst:      int,                // max burst requests
        Rate:       float64,            // spaced rate (requests per second)
    }
    ```

Notes:
- Only Fixed Window supports multiple named quotas simultaneously. See [additional multi-quota documentation](strategies/fixedwindow/MULTI_QUOTA.md).
- When setting a secondary strategy via `WithSecondaryStrategy`, it must advertise `CapSecondary`.
- If a secondary strategy is specified, the primary strategy must not itself be a `CapSecondary`-only secondary in this dual strategy context; the library validates incompatible combinations.
- When using strategies with the limiter wrapper (via `ratelimit.New()`), the `Key` field in strategy configs or `SetKey(string)` calls are ignored. The key is constructed from the limiter's `WithBaseKey` option and the dynamic key provided during `Allow()`/`Peek()` calls. These key configurations are only relevant when using strategies directly without the limiter wrapper.
- Similarly, the `MaxRetries` field in strategy configs or `SetMaxRetries(int)` calls are ignored when using the limiter wrapper. Use `WithMaxRetries(int)` option when creating the limiter instead. The strategy-level retry settings are only relevant when using strategies directly without the limiter wrapper.


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
            AddQuota("minute", 10, time.Minute).    // 10 requests per minute
            AddQuota("hour", 100, time.Hour).       // 100 requests per hour
            Build(),
    ),
)
if err != nil { log.Fatal(err) }
defer limiter.Close()  // Release backend resources
```

**Closing Backends:**
- **With limiter wrapper**: Use `limiter.Close()` (recommended)
- **Direct strategy usage**: Close backend directly with `backend.Close()`

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
    defer store.Close()  // Release backend resources

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
    
    err := cfg.Validate()
    if err != nil {
        panic(err)
    }

    // 4) Perform checks
    // Allow consumes quota and returns per-quota results
    results, err = strat.Allow(ctx, cfg)
    if err != nil {
        panic(err)
    }

    r := results.Default()
    fmt.Printf(
        "allowed=%v remaining=%d reset=%s\n",
        r.Allowed, r.Remaining, r.Reset.Format(time.RFC3339),
    )

    // Peek inspects current state without consuming quota
    peek, err := strat.Peek(ctx, cfg)
    if err != nil {
        panic(err)
    }

    pr := peek.Default()
    fmt.Printf(
        "peek remaining=%d reset=%s\n",
        pr.Remaining, pr.Reset.Format(time.RFC3339),
    )
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

## Memory failover

Memory failover, **disabled by default**, provides automatic failover from the primary storage backend (for example Redis or Postgres) to an in-memory backend when the primary experiences repeated failures. It is enabled via `ratelimit.WithMemoryFailover(...)`, which wraps the backend configured with `ratelimit.WithBackend(...)` in an internal composite backend with a circuit breaker and background health checks.

### Behind the scenes:

Memory failover uses **simplified** circuit breaker and health checker components:

- **Primary backend**: the storage configured with `WithBackend(...)`. Used while healthy.
- **Secondary (in-memory) backend**: a fresh in-memory backend used during primary failures.
- **Circuit breaker**: tracks consecutive failures from the primary. It doesn't categorize error types, any error from the primary counts as a failure toward the threshold. When the threshold is reached, it switches to the in-memory backend.
- **Health checker**: periodically performs a `Get` on the primary to detect when it's healthy again.

By default (when `WithMemoryFailover()` is called with no extra options):

- Failure threshold: **5** consecutive failures before the circuit trips
- Recovery timeout: **30s** before moving from **OPEN** to **HALF-OPEN**
- Health check interval: **10s** between health checks
- Health check timeout: **2s** per health check
- Health check key: `"health-check-key"`

### Configuration

Enable memory failover when constructing the limiter:

- `WithMemoryFailover(opts ...MemoryFailoverOption)`: turns on memory failover for the primary backend.
  - `WithFailureThreshold(threshold int)`: sets how many consecutive primary failures are required before the circuit breaker opens.
  - `WithRecoveryTimeout(timeout time.Duration)`: controls how long the breaker stays OPEN before it retries the primary in HALF-OPEN state.
  - `WithHealthCheckInterval(interval time.Duration)`: how frequently the background health checker probes the primary.
  - `WithHealthCheckTimeout(timeout time.Duration)`: timeout applied to each health check operation.

If you do not provide any options, the defaults listed above are used. `WithMemoryFailover` must be used with a non-memory primary backend that is not already a composite backend; calling it without a configured primary, or with a memory/composite backend, will return an error.

### Usage example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/ajiwo/ratelimit"
    "github.com/ajiwo/ratelimit/backends/redis"
    "github.com/ajiwo/ratelimit/strategies"
    "github.com/ajiwo/ratelimit/strategies/fixedwindow"
)

func main() {
    // Primary backend: Redis
    redisBackend, err := redis.New(redis.Config{
        Addr: "remotehost:6379",
        Password: "top-secret",
        DB: 0,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer redisBackend.Close()

    // Rate limiter with memory failover enabled
    limiter, err := ratelimit.New(
        ratelimit.WithBackend(redisBackend),
        ratelimit.WithMemoryFailover(
            ratelimit.WithFailureThreshold(3),             // Optional, default is 5
            ratelimit.WithRecoveryTimeout(10*time.Second), // Optional, default is 30s
        ),
        ratelimit.WithBaseKey("api"),
        ratelimit.WithPrimaryStrategy(
            fixedwindow.NewConfig().
                AddQuota("default", 5, 3*time.Second).
                Build(),
        ),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer limiter.Close()

    ctx := context.Background()
    userID := "user123"

    var results strategies.Results
    allowed, err := limiter.Allow(ctx, ratelimit.AccessOptions{
        Key: userID,
        Result: &results},
    )
    if err != nil {
        log.Fatal(err)
    }

    res := results.Default()
    fmt.Printf(
        "allowed=%v remaining=%d reset=%s\n",
        allowed, res.Remaining, res.Reset.Format(time.RFC3339),
    )
}
```

### Tradeoffs and when to use it

Memory failover favors **availability over strict global consistency**. The primary and in-memory backends maintain independent state; there is no state synchronization between them. During failover and recovery, some users may effectively see temporary quota resets or partial resets as traffic switches between backends. Over time, normal rate-limiting operations naturally realign state, but the system is not strongly consistent across storage backends.

This might be a good fit when:

- Keeping the service available is more important than enforcing perfectly global rate limits.
- Small, temporary over-allocation of requests is acceptable.
- Basic resilience is desired without running additional infrastructure for state synchronization.

You should **avoid** memory failover when:

- Strict, globally consistent rate limits across all backends are required.
- There is zero tolerance for temporary state fragmentation or quota resets during backend failures.

## License

MIT — see [LICENSE](./LICENSE).
