package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

type Backend struct {
	client *redis.Client
}

func (r *Backend) GetClient() *redis.Client {
	return r.client
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

// CheckAndSet atomically sets key to newValue only if current value matches oldValue
// Returns true if the set was successful, false if value didn't match or key expired
// Empty oldValue means "only set if key doesn't exist"
func (r *Backend) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	// Use Lua script for atomicity
	luaScript := `
	local current = redis.call('GET', KEYS[1])

	-- If oldValue is empty, only set if key doesn't exist
	if ARGV[1] == '' then
		if current == false then
			if ARGV[3] == '0' then
				redis.call('SET', KEYS[1], ARGV[2])
			else
				redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
			end
			return 1
		end
		return 0
	end

	-- Check if current value matches oldValue
	if current == ARGV[1] then
		if ARGV[3] == '0' then
			redis.call('SET', KEYS[1], ARGV[2])
		else
			redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
		end
		return 1
	end

	return 0
	`

	oldStr := oldValue
	newStr := newValue
	var expMs string
	if expiration == 0 {
		expMs = "0"
	} else {
		expMs = fmt.Sprintf("%d", expiration.Milliseconds())
	}

	result, err := r.client.Eval(ctx, luaScript, []string{key}, oldStr, newStr, expMs).Result()
	if err != nil {
		return false, NewEvalFailedError("check-and-set lua script", err)
	}

	return result.(int64) == 1, nil
}
