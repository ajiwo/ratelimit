package composite

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils"
)

// Strategy implements atomic dual-strategy behavior
type Strategy struct {
	storage backends.Backend
}

// New creates a new composite strategy
func New(b backends.Backend, pConfig strategies.Config, sConfig strategies.Config) (*Strategy, error) {
	// Validate inputs
	if pConfig == nil {
		return nil, fmt.Errorf("primary strategy config cannot be nil")
	}
	if sConfig == nil {
		return nil, fmt.Errorf("secondary strategy config cannot be nil")
	}

	// Validate individual configs
	if err := pConfig.Validate(); err != nil {
		return nil, fmt.Errorf("primary config validation failed: %w", err)
	}
	if err := sConfig.Validate(); err != nil {
		return nil, fmt.Errorf("secondary config validation failed: %w", err)
	}

	// Check capabilities
	if !pConfig.Capabilities().Has(strategies.CapPrimary) {
		return nil, fmt.Errorf("primary strategy must support primary capability")
	}
	if !sConfig.Capabilities().Has(strategies.CapSecondary) {
		return nil, fmt.Errorf("secondary strategy must support secondary capability")
	}

	return &Strategy{
		storage: b,
	}, nil
}

// Allow implements atomic dual-strategy logic using composite state
func (cs *Strategy) Allow(ctx context.Context, sci strategies.Config) (strategies.Results, error) {
	cfg, key, maxRetries, err := prepareCompositeForAllow(sci)
	if err != nil {
		return nil, err
	}

	for attempt := range maxRetries {
		results, done, feedback, err := cs.tryAllowOnce(ctx, cfg, key)
		if err != nil {
			return nil, err
		}
		if done {
			return results, nil
		}
		// CAS failed, apply backoff and retry due to contention
		delay := strategies.NextDelay(attempt, feedback)
		if err := utils.SleepOrWait(ctx, delay, 500*time.Millisecond); err != nil {
			return nil, fmt.Errorf("composite allow canceled: %w", err)
		}
	}

	return nil, fmt.Errorf("max retries (%d) exceeded for composite operation", maxRetries)
}

// prepareCompositeForAllow validates and extracts composite config essentials
func prepareCompositeForAllow(sci strategies.Config) (*Config, string, int, error) {
	cfg, ok := sci.(*Config)
	if !ok {
		return nil, "", 0, fmt.Errorf("composite strategy requires CompositeConfig")
	}

	key := cfg.CompositeKey()
	if key == "" {
		return nil, "", 0, fmt.Errorf("composite key not set, call WithKey first")
	}

	maxRetries := cfg.GetMaxRetries()

	return cfg, key, maxRetries, nil
}

// tryAllowOnce executes a single attempt of the composite allow logic.
// Returns:
// - results: final results if decision is determined (denied or committed), or nil if need retry
// - done: true if decision is final (return immediately), false if should retry due to CAS contention
// - duration: time taken for the operation (for backoff calculation), non-zero if retry required
// - err: any error occurred during attempt
func (cs *Strategy) tryAllowOnce(ctx context.Context, cfg *Config, key string) (strategies.Results, bool, time.Duration, error) {
	// Start recording time
	beforeCAS := time.Now()

	// Get current composite state
	oldComposite, err := cs.storage.Get(ctx, key)
	if err != nil {
		return nil, true, 0, fmt.Errorf("failed to get composite state: %w", err)
	}

	// Decode composite state
	oldPrimary, oldSecondary := decodeState(oldComposite)

	// Create single-key adapters seeded with current states
	primaryAdapter := newSingleKeyAdapter(oldPrimary)
	secondaryAdapter := newSingleKeyAdapter(oldSecondary)

	// Create ephemeral strategies bound to adapters
	primaryStrategy, err := strategies.Create(cfg.Primary.ID(), primaryAdapter)
	if err != nil {
		return nil, true, 0, fmt.Errorf("failed to create primary strategy: %w", err)
	}

	secondaryStrategy, err := strategies.Create(cfg.Secondary.ID(), secondaryAdapter)
	if err != nil {
		return nil, true, 0, fmt.Errorf("failed to create secondary strategy: %w", err)
	}

	// Peek primary
	primaryPeekResults, err := primaryStrategy.Peek(ctx, cfg.Primary)
	if err != nil {
		return nil, true, 0, fmt.Errorf("primary strategy peek failed: %w", err)
	}
	if anyDenied(primaryPeekResults) {
		// Denied by primary, final decision without commit
		return prefixResults(primaryPeekResults, "primary_"), true, 0, nil
	}

	// Peek secondary
	secondaryPeekResults, err := secondaryStrategy.Peek(ctx, cfg.Secondary)
	if err != nil {
		return nil, true, 0, fmt.Errorf("secondary strategy peek failed: %w", err)
	}
	if anyDenied(secondaryPeekResults) {
		// Denied by secondary, final decision without commit
		return mergePrefixedResults(
			prefixResults(primaryPeekResults, "primary_"),
			prefixResults(secondaryPeekResults, "secondary_"),
		), true, 0, nil
	}

	// Both allow -> consume quota
	primaryAllowResults, err := primaryStrategy.Allow(ctx, cfg.Primary)
	if err != nil {
		return nil, true, 0, fmt.Errorf("primary strategy allow failed: %w", err)
	}

	secondaryAllowResults, err := secondaryStrategy.Allow(ctx, cfg.Secondary)
	if err != nil {
		return nil, true, 0, fmt.Errorf("secondary strategy allow failed: %w", err)
	}

	// Encode new composite state
	newComposite := encodeState(primaryAdapter.value, secondaryAdapter.value)
	ttl := max(primaryAdapter.expiration, secondaryAdapter.expiration)

	// Atomic commit with CAS
	ok, err := cs.storage.CheckAndSet(ctx, key, oldComposite, newComposite, ttl)
	if err != nil {
		return nil, true, 0, fmt.Errorf("CAS operation failed: %w", err)
	}
	if ok {
		// Success! Merge and return results
		return mergePrefixedResults(
			prefixResults(primaryAllowResults, "primary_"),
			prefixResults(secondaryAllowResults, "secondary_"),
		), true, 0, nil
	}

	// CAS failed -> retry
	return nil, false, time.Since(beforeCAS), nil
}

// prefixResults returns a new results map with all keys prefixed
func prefixResults(in strategies.Results, prefix string) strategies.Results {
	out := make(strategies.Results, len(in))
	for k, v := range in {
		out[prefix+k] = v
	}
	return out
}

// mergePrefixedResults merges two results maps into a new map
func mergePrefixedResults(a, b strategies.Results) strategies.Results {
	out := make(strategies.Results, len(a)+len(b))
	maps.Copy(out, a)
	maps.Copy(out, b)
	return out
}

// anyDenied checks if any result in the map indicates denial
func anyDenied(results strategies.Results) bool {
	for _, result := range results {
		if !result.Allowed {
			return true
		}
	}
	return false
}

// Peek inspects current state without consuming quota using composite state decoding
func (cs *Strategy) Peek(ctx context.Context, sci strategies.Config) (strategies.Results, error) {
	cfg, ok := sci.(*Config)
	if !ok {
		return nil, fmt.Errorf("composite strategy requires CompositeConfig")
	}

	key := cfg.CompositeKey()
	if key == "" {
		return nil, fmt.Errorf("composite key not set, call WithKey first")
	}

	// Get current composite state
	oldComposite, err := cs.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get composite state: %w", err)
	}

	// Decode composite state
	oldPrimary, oldSecondary := decodeState(oldComposite)

	// Create single-key adapters seeded with current states
	primaryAdapter := newSingleKeyAdapter(oldPrimary)
	secondaryAdapter := newSingleKeyAdapter(oldSecondary)

	// Create ephemeral strategies bound to adapters
	primaryStrategy, err := strategies.Create(cfg.Primary.ID(), primaryAdapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create primary strategy: %w", err)
	}

	secondaryStrategy, err := strategies.Create(cfg.Secondary.ID(), secondaryAdapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create secondary strategy: %w", err)
	}

	results := make(strategies.Results)

	// Get primary results
	primaryResults, err := primaryStrategy.Peek(ctx, cfg.Primary)
	if err != nil {
		return nil, fmt.Errorf("failed to get primary results: %w", err)
	}
	for key, result := range primaryResults {
		results["primary_"+key] = result
	}

	// Get secondary results
	secondaryResults, err := secondaryStrategy.Peek(ctx, cfg.Secondary)
	if err != nil {
		return nil, fmt.Errorf("failed to get secondary results: %w", err)
	}
	for key, result := range secondaryResults {
		results["secondary_"+key] = result
	}

	return results, nil
}

// Reset atomically clears composite state using CAS
func (cs *Strategy) Reset(ctx context.Context, sci strategies.Config) error {
	cfg, ok := sci.(*Config)
	if !ok {
		return fmt.Errorf("composite strategy requires CompositeConfig")
	}

	key := cfg.CompositeKey()
	if key == "" {
		return fmt.Errorf("composite key not set, call WithKey first")
	}

	return cs.storage.Delete(ctx, key)
}
