package backends

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ajiwo/ratelimit/backends/scripts"
	"github.com/redis/go-redis/v9"
)

// RedisBackend implements the Backend interface using Redis storage
type RedisBackend struct {
	client *redis.Client
	config BackendConfig
}

// NewRedisBackend creates a new Redis backend
func NewRedisBackend(config BackendConfig) (*RedisBackend, error) {
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

	return &RedisBackend{
		client: client,
		config: config,
	}, nil
}

// Get retrieves rate limit data for the given key
func (r *RedisBackend) Get(ctx context.Context, key string) (*BackendData, error) {
	result, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Key doesn't exist
		}
		return nil, fmt.Errorf("failed to get key from Redis: %w", err)
	}

	var data BackendData
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data from Redis: %w", err)
	}

	return &data, nil
}

// Set stores rate limit data for the given key with optional TTL
func (r *RedisBackend) Set(ctx context.Context, key string, data *BackendData, ttl time.Duration) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data for Redis: %w", err)
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

// Increment atomically checks if incrementing would exceed the limit
func (r *RedisBackend) Increment(ctx context.Context, key string, window time.Duration, limit int64) (int64, bool, error) {
	now := time.Now().Unix()
	windowSeconds := int64(window.Seconds())

	result, err := r.client.Eval(ctx, scripts.CheckAndIncrementScript, []string{key}, windowSeconds, now, limit).Result()
	if err != nil {
		return 0, false, fmt.Errorf("failed to check and increment key in Redis: %w", err)
	}

	// Parse result array [count, incremented]
	resultArray, ok := result.([]any)
	if !ok || len(resultArray) != 2 {
		return 0, false, fmt.Errorf("unexpected result format from Redis check and increment: %T", result)
	}

	count, ok := resultArray[0].(int64)
	if !ok {
		return 0, false, fmt.Errorf("unexpected count type from Redis: %T", resultArray[0])
	}

	incrementedInt, ok := resultArray[1].(int64)
	if !ok {
		return 0, false, fmt.Errorf("unexpected incremented type from Redis: %T", resultArray[1])
	}

	incremented := incrementedInt == 1

	return count, incremented, nil
}

// ConsumeToken atomically checks if a token is available and consumes it
func (r *RedisBackend) ConsumeToken(ctx context.Context, key string, bucketSize int64, refillRate time.Duration, refillAmount int64, window time.Duration) (float64, bool, int64, error) {
	now := time.Now().Unix()
	refillRateSeconds := refillRate.Seconds()
	windowSeconds := window.Seconds()

	result, err := r.client.Eval(ctx, scripts.CheckAndConsumeTokenScript, []string{key}, bucketSize, refillRateSeconds, refillAmount, windowSeconds, now).Result()
	if err != nil {
		return 0, false, 0, fmt.Errorf("failed to check and consume token in Redis: %w", err)
	}

	// Parse result array [tokens, consumed, requests]
	resultArray, ok := result.([]any)
	if !ok || len(resultArray) != 3 {
		return 0, false, 0, fmt.Errorf("unexpected result format from Redis token bucket: %T", result)
	}

	// Parse tokens (can be float)
	var tokens float64
	switch v := resultArray[0].(type) {
	case int64:
		tokens = float64(v)
	case float64:
		tokens = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			tokens = parsed
		} else {
			return 0, false, 0, fmt.Errorf("unexpected tokens type from Redis: %T", resultArray[0])
		}
	default:
		return 0, false, 0, fmt.Errorf("unexpected tokens type from Redis: %T", resultArray[0])
	}

	consumedInt, ok := resultArray[1].(int64)
	if !ok {
		return 0, false, 0, fmt.Errorf("unexpected consumed type from Redis: %T", resultArray[1])
	}
	consumed := consumedInt == 1

	requests, ok := resultArray[2].(int64)
	if !ok {
		return 0, false, 0, fmt.Errorf("unexpected requests type from Redis: %T", resultArray[2])
	}

	return tokens, consumed, requests, nil
}

// Delete removes rate limit data for the given key
func (r *RedisBackend) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key from Redis: %w", err)
	}

	return nil
}

// Close releases any resources held by the backend
func (r *RedisBackend) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// GetStats returns statistics about the Redis backend
func (r *RedisBackend) GetStats(ctx context.Context) (*RedisBackendStats, error) {
	info, err := r.client.Info(ctx, "memory", "keyspace").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get Redis info: %w", err)
	}

	// Parse basic stats from Redis INFO command
	stats := &RedisBackendStats{
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

// RedisBackendStats holds statistics about the Redis backend
type RedisBackendStats struct {
	Address   string
	DB        int
	PoolSize  int
	Info      string
	PoolStats RedisPoolStats
}

// Ping tests the Redis connection
func (r *RedisBackend) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// FlushDB clears all keys in the current database (use with caution!)
func (r *RedisBackend) FlushDB(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

// Keys returns all keys matching the pattern (use with caution in production!)
func (r *RedisBackend) Keys(ctx context.Context, pattern string) ([]string, error) {
	return r.client.Keys(ctx, pattern).Result()
}
