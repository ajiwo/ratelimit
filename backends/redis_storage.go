package backends

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStorage implements the Storage interface using Redis
type RedisStorage struct {
	client *redis.Client
	config BackendConfig
}

// NewRedisStorage creates a new Redis storage backend
func NewRedisStorage(config BackendConfig) (*RedisStorage, error) {
	if config.RedisAddress == "" {
		return nil, fmt.Errorf("redis address cannot be empty")
	}

	if config.RedisPoolSize <= 0 {
		config.RedisPoolSize = 10 // Default pool size
	}

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:         config.RedisAddress,
		Password:     config.RedisPassword,
		DB:           config.RedisDB,
		PoolSize:     config.RedisPoolSize,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		DialTimeout:  5 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisStorage{
		client: client,
		config: config,
	}, nil
}

// Get retrieves state data for the given key
func (r *RedisStorage) Get(ctx context.Context, key string) (*State, error) {
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Key doesn't exist
		}
		return nil, fmt.Errorf("failed to get key from Redis: %w", err)
	}

	var state State
	if err := json.Unmarshal([]byte(result), &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state from Redis: %w", err)
	}

	return &state, nil
}

// Set stores state data for the given key with optional TTL
func (r *RedisStorage) Set(ctx context.Context, key string, state *State, ttl time.Duration) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}

	jsonData, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state for Redis: %w", err)
	}

	if ttl > 0 {
		err = r.client.Set(ctx, key, jsonData, ttl).Err()
	} else {
		err = r.client.Set(ctx, key, jsonData, 0).Err()
	}

	if err != nil {
		return fmt.Errorf("failed to set key in Redis: %w", err)
	}

	return nil
}

// CompareAndSet atomically updates state if it matches expected state
func (r *RedisStorage) CompareAndSet(ctx context.Context, key string, expected *State, new *State, ttl time.Duration) (bool, error) {
	// Use Redis WATCH/MULTI/EXEC for atomic compare-and-set
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		// Get current value
		current, err := tx.Get(ctx, key).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return fmt.Errorf("failed to get current value: %w", err)
		}

		// If no expected state, this is a create operation
		if expected == nil {
			if current != "" {
				return fmt.Errorf("key already exists") // CAS failed
			}
		} else {
			// If expected state provided, check if current matches
			if current == "" {
				return fmt.Errorf("key does not exist") // CAS failed
			}

			var currentState State
			if err := json.Unmarshal([]byte(current), &currentState); err != nil {
				return fmt.Errorf("failed to unmarshal current state: %w", err)
			}

			// Simple comparison - in production you might want more sophisticated comparison
			if currentState.Counter != expected.Counter ||
				len(currentState.Timestamps) != len(expected.Timestamps) ||
				len(currentState.Values) != len(expected.Values) {
				return fmt.Errorf("state does not match expected") // CAS failed
			}
		}

		// Marshal new state
		newData, err := json.Marshal(new)
		if err != nil {
			return fmt.Errorf("failed to marshal new state: %w", err)
		}

		// Set new value
		if ttl > 0 {
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, newData, ttl)
				return nil
			})
		} else {
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, newData, 0)
				return nil
			})
		}

		return err
	}, key)

	if err != nil {
		if errors.Is(err, redis.TxFailedErr) {
			return false, nil // Transaction failed due to watched key change
		}
		return false, fmt.Errorf("failed to execute compare-and-set: %w", err)
	}

	return true, nil
}

// Delete removes state data for the given key
func (r *RedisStorage) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key from Redis: %w", err)
	}

	return nil
}

// Close releases any resources held by the storage backend
func (r *RedisStorage) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// GetStats returns statistics about the Redis storage
func (r *RedisStorage) GetStats(ctx context.Context) (*RedisStorageStats, error) {
	info, err := r.client.Info(ctx, "memory", "keyspace").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis info: %w", err)
	}

	stats := &RedisStorageStats{
		Address:  r.config.RedisAddress,
		DB:       r.config.RedisDB,
		PoolSize: r.config.RedisPoolSize,
		Info:     info,
	}

	// Get connection pool stats
	poolStats := r.client.PoolStats()
	stats.PoolStats = RedisPoolStats{
		Hits:       poolStats.Hits,
		Misses:     poolStats.Misses,
		Timeouts:   poolStats.Timeouts,
		TotalConns: poolStats.TotalConns,
		IdleConns:  poolStats.IdleConns,
		StaleConns: poolStats.StaleConns,
	}

	return stats, nil
}

// RedisStorageStats holds statistics about the Redis storage backend
type RedisStorageStats struct {
	Address   string
	DB        int
	PoolSize  int
	Info      string
	PoolStats RedisPoolStats
}

// RedisPoolStats holds Redis connection pool statistics
type RedisPoolStats struct {
	Hits       uint32
	Misses     uint32
	Timeouts   uint32
	TotalConns uint32
	IdleConns  uint32
	StaleConns uint32
}

// Ping tests the Redis connection
func (r *RedisStorage) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// FlushDB clears all keys in the current database (use with caution!)
func (r *RedisStorage) FlushDB(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

// Keys returns all keys matching the pattern (use with caution in production!)
func (r *RedisStorage) Keys(ctx context.Context, pattern string) ([]string, error) {
	return r.client.Keys(ctx, pattern).Result()
}
