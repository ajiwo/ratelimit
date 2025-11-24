package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	_ "embed"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

const checkAndSetSHA = "31f0e6b6c096d994958b631fc6251ffa77352e89"

//go:embed cns.lua
var cnsScript string

type Backend struct {
	client redis.UniversalClient
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
		return NewEvalFailedError("load check-and-set script", err)
	}
	if sha != checkAndSetSHA {
		return ErrScriptSHAInvalid
	}
	return nil
}

// New initializes a new RedisStorage with the given configuration.
func New(config Config) (*Backend, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})

	if _, err := client.Ping(context.Background()).Result(); err != nil {
		return nil, NewConnectionFailedError(config.Addr, err)
	}

	return &Backend{client: client}, nil
}

// NewWithClient initializes a new Backend with a pre-configured Redis universal client.
//
// The client is assumed to be already connected and ready for use.
func NewWithClient(client redis.UniversalClient) *Backend {
	return &Backend{client: client}
}

func (r *Backend) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // Key doesn't exist, return empty string with no error
	}
	if err != nil {
		return "", NewGetFailedError(key, err)
	}
	return val, nil
}

func (r *Backend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	if err := r.client.Set(ctx, key, value, expiration).Err(); err != nil {
		return NewSetFailedError(key, err)
	}
	return nil
}

func (r *Backend) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return NewDeleteFailedError(key, err)
	}
	return nil
}

func (r *Backend) Close() error {
	if err := r.client.Close(); err != nil {
		return NewCloseFailedError(err)
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
				return false, NewEvalFailedError("reload check-and-set script", loadErr)
			}
			result, err = r.client.
				EvalSha(ctx, checkAndSetSHA, []string{key}, oldStr, newStr, expMs).
				Result()
			if err != nil {
				return false, NewEvalFailedError("check-and-set cached script", err)
			}
		} else {
			return false, NewEvalFailedError("check-and-set cached script", err)
		}
	}

	return result.(int64) == 1, nil
}
