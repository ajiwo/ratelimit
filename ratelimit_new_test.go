package ratelimit

import (
	"errors"
	"testing"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// factory that returns provided strategy instance
func registerMockStrategy(t *testing.T, id strategies.StrategyID, s strategies.Strategy) {
	t.Helper()
	strategies.Register(id, func(_ backends.Backend) strategies.Strategy { // backend is not used by mocks
		return s
	})
}

func TestNew_And_Composites(t *testing.T) {
	mb := &mockBackendOne{}

	// prepare factories to be used by newRateLimiter
	primCfg := mockStrategyConfig{id: strategies.StrategyTokenBucket, caps: strategies.CapPrimary}
	secCfg := mockStrategyConfig{id: strategies.StrategyGCRA, caps: strategies.CapSecondary}

	primary := &mockStrategyOne{getRes: strategies.Results{"p": {Allowed: true}}, allowRes: strategies.Results{"p": {Allowed: true}}}
	secondary := &mockStrategyOne{getRes: strategies.Results{"s": {Allowed: true}}, allowRes: strategies.Results{"s": {Allowed: true}}}

	registerMockStrategy(t, primCfg.id, primary)
	registerMockStrategy(t, secCfg.id, secondary)

	rl, err := New(
		WithBackend(mb),
		WithBaseKey("base"),
		WithMaxRetries(3),
		WithPrimaryStrategy(primCfg),
		WithSecondaryStrategy(secCfg),
	)
	require.NoError(t, err, "unexpected error creating limiter")

	// Ensure composite flow works and returns results without errors
	allowed, err := rl.Allow(t.Context(), AccessOptions{})
	require.NoError(t, err, "composite allow error")
	require.True(t, allowed, "expected allowed when both strategies allow")

	// Close should delegate to backend
	require.NoError(t, rl.Close(), "close error")
	assert.True(t, mb.closed, "backend Close not called")
}

func TestNew_SingleStrategy_ErrorPaths(t *testing.T) {
	// Validate failing option should bubble up
	badOpt := func(c *Config) error { return errors.New("bad opt") }
	_, err := New(badOpt)
	require.Error(t, err, "expected error when option fails")

	// Invalid config should error
	_, err = newRateLimiter(Config{})
	require.Error(t, err, "expected error for invalid config in newRateLimiter")

	// Strategy.Create error when unregistered id
	mb := &mockBackendOne{}
	primCfg := mockStrategyConfig{id: 255, caps: strategies.CapPrimary} // unregistered id
	_, err = newRateLimiter(Config{BaseKey: "base", Storage: mb, PrimaryConfig: primCfg})
	require.Error(t, err, "expected error when creating primary strategy fails")
}

func TestAllow_ErrorAndDenialPaths(t *testing.T) {
	mb := &mockBackendOne{}
	primCfg := mockStrategyConfig{id: strategies.StrategyTokenBucket, caps: strategies.CapPrimary}

	// Register factory returning a strategy that errors
	failing := &mockStrategyOne{allowErr: errors.New("boom")}
	registerMockStrategy(t, primCfg.id, failing)

	rl, err := New(WithBackend(mb), WithBaseKey("base"), WithPrimaryStrategy(primCfg))
	require.NoError(t, err, "unexpected error")

	ok, err := rl.Allow(t.Context(), AccessOptions{})
	require.Error(t, err, "expected error from strategy allow")
	assert.False(t, ok, "expected ok to be false when strategy errors")

	// Now make it return mixed results (one false) so Allow returns false
	denying := &mockStrategyOne{allowRes: strategies.Results{"a": {Allowed: true}, "b": {Allowed: false}}}
	registerMockStrategy(t, primCfg.id, denying)

	rl, err = New(WithBackend(mb), WithBaseKey("base"), WithPrimaryStrategy(primCfg))
	require.NoError(t, err, "unexpected error")

	ok, err = rl.Allow(t.Context(), AccessOptions{})
	require.NoError(t, err, "unexpected error")
	assert.False(t, ok, "expected not allowed when any quota denies")
}

func TestPeekAndReset_ErrorPaths(t *testing.T) {
	mb := &mockBackendOne{}
	primCfg := mockStrategyConfig{id: strategies.StrategyTokenBucket, caps: strategies.CapPrimary}

	// Strategy GetResult fails
	st := &mockStrategyOne{getErr: errors.New("bad get")}
	registerMockStrategy(t, primCfg.id, st)

	rl, err := New(WithBackend(mb), WithBaseKey("base"), WithPrimaryStrategy(primCfg))
	require.NoError(t, err, "unexpected error")

	_, err = rl.Peek(t.Context(), AccessOptions{})
	require.Error(t, err, "expected error from Peek")

	// Reset fails
	st = &mockStrategyOne{resetErr: errors.New("bad reset")}
	registerMockStrategy(t, primCfg.id, st)
	rl, err = New(WithBackend(mb), WithBaseKey("base"), WithPrimaryStrategy(primCfg))
	require.NoError(t, err, "unexpected error")

	err = rl.Reset(t.Context(), AccessOptions{})
	require.Error(t, err, "expected error from Reset")
}
