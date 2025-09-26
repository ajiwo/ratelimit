package redis

import (
	"github.com/ajiwo/ratelimit/backends"
)

func init() {
	backends.Register("redis", func(config any) (backends.Backend, error) {
		redisConfig, ok := config.(backends.RedisConfig)
		if !ok {
			return nil, backends.ErrInvalidConfig
		}
		if redisConfig.Addr == "" {
			return nil, backends.ErrInvalidConfig
		}
		return NewRedisStorage(RedisConfig{
			Addr:     redisConfig.Addr,
			Password: redisConfig.Password,
			DB:       redisConfig.DB,
			PoolSize: redisConfig.PoolSize,
		})
	})
}
