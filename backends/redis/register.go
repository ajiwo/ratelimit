package redis

import (
	"github.com/ajiwo/ratelimit/backends"
)

func init() {
	backends.Register("redis", func(config any) (backends.Backend, error) {
		redisConfig, ok := config.(Config)
		if !ok {
			return nil, backends.ErrInvalidConfig
		}
		if redisConfig.Addr == "" {
			return nil, backends.ErrInvalidConfig
		}
		return New(Config{
			Addr:     redisConfig.Addr,
			Password: redisConfig.Password,
			DB:       redisConfig.DB,
			PoolSize: redisConfig.PoolSize,
		})
	})
}
