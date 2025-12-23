package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr     string // Redis server address (host:port)
	Password string // Redis server password
	DB       int    // Redis database number
	PoolSize int    // Connection pool size
	// RedisURL is a connection string in Redis URL format that provides all connection parameters.
	//
	// When set, it takes precedence over individual Addr, Password, DB, and PoolSize fields.
	// Format examples:
	//   - "redis://user:password@localhost:6789/3?dial_timeout=3s&pool_size=10"
	//   - "unix://user:password@/path/to/redis.sock?db=1"
	// Individual fields can be used to override URL parameters if explicitly set.
	RedisURL string
	// ConnErrorStrings contains string patterns to identify connectivity-related errors.
	//
	// If nil, the default patterns from connErrorStrings are used.
	// These patterns help distinguish temporary connectivity issues from operational errors
	// like "NOSCRIPT" or "WRONGTYPE".
	ConnErrorStrings []string
}

const checkAndSetSHA = "31f0e6b6c096d994958b631fc6251ffa77352e89"

//go:embed cns.lua
var cnsScript string

type Backend struct {
	client           redis.UniversalClient
	connErrorStrings []string
}

func (r *Backend) GetClient() redis.UniversalClient {
	return r.client
}

// loadCheckAndSetScript loads and caches the CheckAndSet Lua script in Redis.
//
// The script (cns.lua) implements atomic compare-and-swap semantics:
// - KEYS[1]: Redis storage key
// - ARGV[1]: Expected current value (oldValue)
// - ARGV[2]: New value to set (newValue)
// - ARGV[3]: Expiration in milliseconds ("0" = no expiration)
func (r *Backend) loadCheckAndSetScript(ctx context.Context) error {
	sha, err := r.client.ScriptLoad(ctx, cnsScript).Result()
	if err != nil {
		return r.maybeConnError("redis:ScriptLoad",
			fmt.Errorf("failed to evaluate lua script: %w", err))
	}
	if sha != checkAndSetSHA {
		return fmt.Errorf("invalid script SHA hash for lua script")
	}
	return nil
}

// New initializes a new RedisStorage with the given configuration.
func New(config Config) (*Backend, error) {
	var client redis.UniversalClient

	if config.RedisURL != "" {
		// Parse the Redis URL to get configuration options
		options, err := redis.ParseURL(config.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
		}

		// Override with explicit config values if provided
		if config.Addr != "" {
			options.Addr = config.Addr
		}
		if config.Password != "" {
			options.Password = config.Password
		}
		if config.DB != 0 {
			options.DB = config.DB
		}
		if config.PoolSize != 0 {
			options.PoolSize = config.PoolSize
		}

		client = redis.NewClient(options)
	} else {
		// Use individual configuration fields
		client = redis.NewClient(&redis.Options{
			Addr:     config.Addr,
			Password: config.Password,
			DB:       config.DB,
			PoolSize: config.PoolSize,
		})
	}

	// Use custom patterns if provided, otherwise fall back to defaults
	patterns := config.ConnErrorStrings
	if patterns == nil {
		patterns = connErrorStrings
	}

	if _, err := client.Ping(context.Background()).Result(); err != nil {
		return nil, backends.NewHealthError("redis:Ping",
			fmt.Errorf("redis ping failed: %w", err))
	}

	return &Backend{
		client:           client,
		connErrorStrings: patterns,
	}, nil
}

// NewWithClient initializes a new Backend with a pre-configured Redis universal client.
//
// The client is assumed to be already connected and ready for use.
func NewWithClient(client redis.UniversalClient) *Backend {
	return &Backend{
		client:           client,
		connErrorStrings: connErrorStrings, // Use default patterns
	}
}

func (r *Backend) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // Key doesn't exist, return empty string with no error
	}
	if err != nil {
		return "", fmt.Errorf("failed to get key '%s': %w", key, err)
	}
	return val, nil
}

func (r *Backend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	if err := r.client.Set(ctx, key, value, expiration).Err(); err != nil {
		return fmt.Errorf("failed to set key '%s': %w", key, err)
	}
	return nil
}

func (r *Backend) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete key '%s': %w", key, err)
	}
	return nil
}

func (r *Backend) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("failed to close redis connection: %w", err)
	}
	return nil
}

// CheckAndSet atomically sets key to newValue only if current value matches oldValue.
// This operation provides compare-and-swap (CAS) semantics for implementing optimistic locking.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - key: The storage key to operate on
//   - oldValue: Expected current value. Use empty string "" for "set if not exists" semantics
//   - newValue: New value to set if the current value matches oldValue
//   - expiration: Time-to-live for the key. Use 0 for no expiration
//
// Returns:
//   - bool: true if the CAS succeeded (value was set), false if the compare failed and no write occurred
//   - error: Any storage-related error (not including a compare mismatch)
//
// Behavior:
//   - If oldValue is "", the operation succeeds only if the key does not exist (or is expired)
//   - If oldValue matches the current value, the key is updated to newValue
//   - Expired keys are treated as non-existent for comparison purposes
//   - All values are stored and compared as strings
//
// Caller contract:
//   - A (false, nil) return indicates the compare condition did not match (e.g., another writer won the race
//     or the key already exists when using "set if not exists"). This is not an error. Callers may safely
//     reload state and retry with backoff according to their contention policy.
//   - A non-nil error indicates a storage/backend failure and should not be retried blindly.

func (r *Backend) CheckAndSet(ctx context.Context, key, oldValue, newValue string, expiration time.Duration) (bool, error) {
	oldStr := oldValue
	newStr := newValue
	var expMs string
	if expiration == 0 {
		expMs = "0"
	} else {
		expMs = fmt.Sprintf("%d", expiration.Milliseconds())
	}

	result, err := r.client.
		EvalSha(ctx, checkAndSetSHA, []string{key}, oldStr, newStr, expMs).
		Result()
	if err != nil {
		// If script was not cached or flushed from Redis, load it and retry
		if strings.Contains(err.Error(), "NOSCRIPT") {
			if loadErr := r.loadCheckAndSetScript(ctx); loadErr != nil {
				return false, fmt.Errorf("failed to evaluate lua script: %w", loadErr)
			}
			result, err = r.client.
				EvalSha(ctx, checkAndSetSHA, []string{key}, oldStr, newStr, expMs).
				Result()
			if err != nil {
				return false, r.maybeConnError("redis:CheckAndSet",
					fmt.Errorf("failed to evaluate lua script: %w", err))
			}
		} else {
			return false, r.maybeConnError("redis:CheckAndSet",
				fmt.Errorf("failed to evaluate lua script: %w", err))
		}
	}

	return result.(int64) == 1, nil
}

// maybeConnError checks if the error is a connectivity issue and wraps it as a health error.
//
// For Redis, we consider connection timeouts, connection refused, and network errors as health issues.
// Operational errors like NOSCRIPT are not considered health errors.
func (r *Backend) maybeConnError(op string, err error) error {
	return backends.MaybeConnError(op, err, r.connErrorStrings)
}
