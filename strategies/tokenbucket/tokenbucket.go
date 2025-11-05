package tokenbucket

import (
	"context"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/tokenbucket/internal"
)

type Strategy struct {
	storage backends.Backend
}

func New(storage backends.Backend) *Strategy {
	return &Strategy{storage: storage}
}

type TokenBucket = internal.TokenBucket

func (t *Strategy) Allow(ctx context.Context, config strategies.Config) (map[string]strategies.Result, error) {
	tokenConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, t.storage, tokenConfig, internal.TryUpdate)
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

func (t *Strategy) Peek(ctx context.Context, config strategies.Config) (map[string]strategies.Result, error) {
	tokenConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, t.storage, tokenConfig, internal.ReadOnly)
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

func (t *Strategy) Reset(ctx context.Context, config strategies.Config) error {
	tokenConfig, ok := config.(Config)
	if !ok {
		return ErrInvalidConfig
	}

	return t.storage.Delete(ctx, tokenConfig.Key)
}
