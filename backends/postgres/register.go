package postgres

import (
	"github.com/ajiwo/ratelimit/backends"
)

func init() {
	backends.Register("postgres", func(config any) (backends.Backend, error) {
		pgConfig, ok := config.(Config)
		if !ok {
			return nil, backends.ErrInvalidConfig
		}
		if pgConfig.ConnString == "" {
			return nil, backends.ErrInvalidConfig
		}
		return New(Config{
			ConnString: pgConfig.ConnString,
			MaxConns:   pgConfig.MaxConns,
			MinConns:   pgConfig.MinConns,
		})
	})
}
