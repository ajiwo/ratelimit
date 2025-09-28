package postgres

import (
	"github.com/ajiwo/ratelimit/backends"
)

func init() {
	backends.Register("postgres", func(config any) (backends.Backend, error) {
		pgConfig, ok := config.(PostgresConfig)
		if !ok {
			return nil, backends.ErrInvalidConfig
		}
		if pgConfig.ConnString == "" {
			return nil, backends.ErrInvalidConfig
		}
		return New(PostgresConfig{
			ConnString: pgConfig.ConnString,
			MaxConns:   pgConfig.MaxConns,
			MinConns:   pgConfig.MinConns,
		})
	})
}
