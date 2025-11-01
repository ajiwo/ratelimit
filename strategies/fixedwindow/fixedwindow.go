package fixedwindow

import (
	"context"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/strategies/fixedwindow/internal"
)

// Strategy implements the fixed window rate limiting algorithm
type Strategy struct {
	storage backends.Backend
}

// New creates a new fixed window strategy
func New(storage backends.Backend) *Strategy {
	return &Strategy{
		storage: storage,
	}
}

// FixedWindow is exported for backward compatibility
type FixedWindow = internal.FixedWindow

// Allow checks if a request is allowed and returns detailed statistics
func (f *Strategy) Allow(ctx context.Context, config strategies.StrategyConfig) (strategies.Results, error) {
	fixedConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, f.storage, fixedConfig, internal.TryUpdate)
	if err != nil {
		return nil, err
	}

	return convertResults(res), nil
}

// Peek inspects current state without consuming quota
func (f *Strategy) Peek(ctx context.Context, config strategies.StrategyConfig) (strategies.Results, error) {
	fixedConfig, ok := config.(Config)
	if !ok {
		return nil, ErrInvalidConfig
	}

	res, err := internal.Allow(ctx, f.storage, fixedConfig, internal.ReadOnly)
	if err != nil {
		return nil, err
	}

	return convertResults(res), nil
}

// Reset resets the rate limit counter for the given key
func (f *Strategy) Reset(ctx context.Context, config strategies.StrategyConfig) error {
	fixedConfig, ok := config.(Config)
	if !ok {
		return ErrInvalidConfig
	}

	return internal.Reset(ctx, fixedConfig, f.storage)
}

// convertResults converts internal.Result map to strategies.Result map
func convertResults(internalResults map[string]internal.Result) strategies.Results {
	results := make(strategies.Results, len(internalResults))
	for name, res := range internalResults {
		results[name] = strategies.Result{
			Allowed:   res.Allowed,
			Remaining: res.Remaining,
			Reset:     res.Reset,
		}
	}
	return results
}
