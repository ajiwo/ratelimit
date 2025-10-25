package leakybucket

import (
	"context"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/leakybucket/internal"
)

// Strategy implements the leaky bucket rate limiting algorithm
type Strategy struct {
	storage backends.Backend
}

// New creates a new leaky bucket strategy
func New(storage backends.Backend) *Strategy {
	return &Strategy{
		storage: storage,
	}
}

// Allow checks if a request is allowed and returns detailed statistics
func (l *Strategy) Allow(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	lbConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, l.storage, lbConfig, internal.TryUpdate)
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
func (l *Strategy) Peek(ctx context.Context, config strategies.StrategyConfig) (map[string]strategies.Result, error) {
	lbConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, l.storage, lbConfig, internal.ReadOnly)
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

// Reset resets the leaky bucket state for the given key
func (l *Strategy) Reset(ctx context.Context, config strategies.StrategyConfig) error {
	lbConfig, ok := config.(Config)
	if !ok {
		return ErrInvalidConfig
	}

	return l.storage.Delete(ctx, lbConfig.Key)
}
