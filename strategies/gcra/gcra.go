package gcra

import (
	"context"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/gcra/internal"
)

// Strategy implements the GCRA rate limiting algorithm
type Strategy struct {
	storage backends.Backend
}

// New creates a new GCRA strategy
func New(storage backends.Backend) *Strategy {
	return &Strategy{
		storage: storage,
	}
}

// Allow checks if a request is allowed and returns detailed statistics
func (g *Strategy) Allow(ctx context.Context, config strategies.Config) (map[string]strategies.Result, error) {
	gcraConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, g.storage, gcraConfig, internal.TryUpdate)
	if err != nil {
		return nil, err
	}
	return map[string]strategies.Result{
		"default": {
			Allowed:   res.Allowed,
			Remaining: res.Remaining,
			Reset:     res.Reset,
		},
	}, nil
}

// Peek inspects current state without consuming quota
func (g *Strategy) Peek(ctx context.Context, config strategies.Config) (map[string]strategies.Result, error) {
	gcraConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, g.storage, gcraConfig, internal.ReadOnly)
	if err != nil {
		return nil, err
	}
	return map[string]strategies.Result{
		"default": {
			Allowed:   res.Allowed,
			Remaining: res.Remaining,
			Reset:     res.Reset,
		},
	}, nil
}

// Reset resets the GCRA state for the given key
func (g *Strategy) Reset(ctx context.Context, config strategies.Config) error {
	gcraConfig, ok := config.(Config)
	if !ok {
		return ErrInvalidConfig
	}

	return g.storage.Delete(ctx, gcraConfig.Key)
}
