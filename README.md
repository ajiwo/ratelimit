# ratelimit

A rate limiting library for Go applications with multiple backends and algorithm strategies.

## Installation

```bash
go get github.com/ajiwo/ratelimit
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/ajiwo/ratelimit/backends"
    "github.com/ajiwo/ratelimit/strategies"
)

func main() {
    // Create memory backend
    backendConfig := backends.BackendConfig{
        Type:            "memory",
        CleanupInterval: 5 * time.Minute,
        MaxKeys:         10000,
    }

    backend, err := backends.NewBackend(backendConfig)
    if err != nil {
        panic(err)
    }
    defer backend.Close()

    // Create token bucket strategy
    strategyConfig := strategies.StrategyConfig{
        Type:         "token_bucket",
        RefillRate:   time.Second,
        RefillAmount: 5,
        BucketSize:   20,
    }

    strategy, err := strategies.NewStrategy(strategyConfig)
    if err != nil {
        panic(err)
    }

    // Test rate limiting
    userID := "user:123"
    
    for i := range 5 {
        result, err := strategy.Allow(context.Background(), backend, userID, 20, time.Minute)
        if err != nil {
            panic(err)
        }

        if result.Allowed {
            fmt.Printf("Request %d: ALLOWED (remaining: %d)\n", i+1, result.Remaining)
        } else {
            fmt.Printf("Request %d: BLOCKED\n", i+1)
        }
    }
}
```

## Strategies

### Token Bucket

The token bucket algorithm provides a flexible rate limiting approach that allows for bursts of traffic while maintaining an average rate limit.

```go
strategyConfig := strategies.StrategyConfig{
    Type:         "token_bucket",
    RefillRate:   time.Second,     // Refill tokens every second
    RefillAmount: 5,               // Add 5 tokens per refill
    BucketSize:   20,              // Maximum 20 tokens in bucket
}
```

### Fixed Window

The fixed window algorithm divides time into fixed intervals and allows a maximum number of requests per interval.

```go
strategyConfig := strategies.StrategyConfig{
    Type:           "fixed_window",
    WindowDuration: 10 * time.Second, // Fixed window of 10 seconds
}
```

## Backends

### Memory

In-memory backend for single-instance applications.

```go
backendConfig := backends.BackendConfig{
    Type:            "memory",
    CleanupInterval: 5 * time.Minute,
    MaxKeys:         10000,
}
```

### Redis

Redis backend for distributed applications.

```go
backendConfig := backends.BackendConfig{
    Type:          "redis",
    RedisAddress:  "localhost:6379",
    RedisPassword: "",
    RedisDB:       0,
    RedisPoolSize: 10,
}
```

## Examples

See [examples/main.go](examples/main.go) for comprehensive usage examples.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.