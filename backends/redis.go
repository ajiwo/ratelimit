package backends

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

type RedisStorage struct {
	client *redis.Client
}

func (r *RedisStorage) GetClient() *redis.Client {
	return r.client
}

func NewRedisStorage(config RedisConfig) (*RedisStorage, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})

	if _, err := client.Ping(context.Background()).Result(); err != nil {
		return nil, err
	}

	return &RedisStorage{client: client}, nil
}

func (r *RedisStorage) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // Key doesn't exist, return empty string with no error
	}
	return val, err
}

func (r *RedisStorage) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

func (r *RedisStorage) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisStorage) Close() error {
	return r.client.Close()
}
