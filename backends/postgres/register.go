package postgres

import (
	"github.com/ajiwo/ratelimit/backends"
)

func init() {
	backends.Register("postgres", func(config any) (backends.Backend, error) {
		pgConfig, ok := config.(backends.PostgresConfig)
		if !ok {
			return nil, backends.ErrInvalidConfig
		}
		if pgConfig.ConnString == "" {
			return nil, backends.ErrInvalidConfig
		}
		return NewPostgresStorage(PostgresConfig{
			ConnString: pgConfig.ConnString,
			MaxConns:   pgConfig.MaxConns,
			MinConns:   pgConfig.MinConns,
		})
	})
}
