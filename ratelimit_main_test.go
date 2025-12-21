package ratelimit

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/internal/strategies/composite"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ backends.Backend = (*mockBackendOne)(nil)

type mockBackendOne struct {
	data        map[string]string
	closed      bool
	closeErr    error
	setCalls    int
	getCalls    int
	casCalls    int
	deleteCalls int
}

func (m *mockBackendOne) Get(ctx context.Context, key string) (string, error) {
	m.getCalls++
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return "", nil
}

func (m *mockBackendOne) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	m.setCalls++
	if m.data == nil {
		m.data = make(map[string]string)
	}
	m.data[key] = value
	return nil
}

func (m *mockBackendOne) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	m.casCalls++
	return true, nil
}

func (m *mockBackendOne) Delete(ctx context.Context, key string) error {
	m.deleteCalls++
	delete(m.data, key)
	return nil
}

func (m *mockBackendOne) Close() error {
	m.closed = true
	return m.closeErr
}

var _ strategies.Strategy = (*mockStrategyOne)(nil)

// strategy that records the last config and returns preconfigured results/errors.
type mockStrategyOne struct {
	lastConfig strategies.Config
	allowRes   strategies.Results
	allowErr   error
	getRes     strategies.Results
	getErr     error
	resetErr   error
}

func (m *mockStrategyOne) Allow(ctx context.Context, cfg strategies.Config) (strategies.Results, error) {
	m.lastConfig = cfg
	return m.allowRes, m.allowErr
}

func (m *mockStrategyOne) Peek(ctx context.Context, cfg strategies.Config) (strategies.Results, error) {
	m.lastConfig = cfg
	return m.getRes, m.getErr
}

func (m *mockStrategyOne) Reset(ctx context.Context, cfg strategies.Config) error {
	m.lastConfig = cfg
	return m.resetErr
}

// mockStrategyConfig implements strategies.StrategyConfig

type mockStrategyConfig struct {
	id         strategies.ID
	caps       strategies.CapabilityFlags
	key        string
	valid      error
	maxRetries int
}

func (m mockStrategyConfig) Validate() error                          { return m.valid }
func (m mockStrategyConfig) ID() strategies.ID                        { return m.id }
func (m mockStrategyConfig) Capabilities() strategies.CapabilityFlags { return m.caps }

func (m mockStrategyConfig) WithKey(key string) strategies.Config {
	m.key = key
	return m
}
func (m mockStrategyConfig) GetMaxRetries() int { return m.maxRetries }
func (m mockStrategyConfig) WithMaxRetries(retries int) strategies.Config {
	m.maxRetries = retries
	return m
}

func TestWithPrimaryAndSecondaryStrategyOptions(t *testing.T) {
	cfg := &Config{}

	// primary cannot be nil
	require.Error(t, WithPrimaryStrategy(nil)(cfg), "expected error when primary is nil")

	// secondary cannot be nil and must have CapSecondary
	require.Error(t, WithSecondaryStrategy(nil)(cfg), "expected error when secondary is nil")

	secBad := mockStrategyConfig{caps: strategies.CapPrimary}
	require.Error(t, WithSecondaryStrategy(secBad)(cfg), "expected error when secondary lacks CapSecondary")

	// ok when has CapSecondary
	secGood := mockStrategyConfig{caps: strategies.CapSecondary}
	require.NoError(t, WithSecondaryStrategy(secGood)(cfg), "unexpected error")
}

func TestWithBackend_ClosesPrevious(t *testing.T) {
	var err error
	cfg := &Config{}

	mb1 := &mockBackendOne{}
	err = WithBackend(mb1)(cfg)
	require.NoError(t, err, "unexpected error")
	assert.Equal(t, mb1, cfg.Storage, "backend not set")

	// swapping should close previous
	mb2 := &mockBackendOne{}
	err = WithBackend(mb2)(cfg)
	require.NoError(t, err, "unexpected error")
	assert.True(t, mb1.closed, "expected previous backend closed")
	assert.Equal(t, mb2, cfg.Storage, "backend not swapped")

	// nil backend should error
	err = WithBackend(nil)(cfg)
	require.Error(t, err, "expected error for nil backend")

	// closing error propagates
	cfg = &Config{Storage: &mockBackendOne{closeErr: errors.New("boom")}}
	err = WithBackend(&mockBackendOne{})(cfg)
	require.Error(t, err, "expected error when closing existing backend fails")
}

func TestValidateKey(t *testing.T) {
	cases := []struct {
		key string
		ok  bool
	}{
		{"a", true},
		{"abc-123_:@.", true},
		{"", false},
		{"this-key-is-way-too-long-because-it-exceeds-sixty-four-characters-xxxx", false},
		{"space not allowed", false},
		{"ünicode", false},
	}
	for _, tc := range cases {
		err := validateKey(tc.key, "dynamic key")
		if tc.ok {
			assert.NoError(t, err, "expected ok for %q", tc.key)
		} else {
			assert.Error(t, err, "expected error for %q", tc.key)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	var err error
	var cfg Config
	mb := &mockBackendOne{}
	primary := mockStrategyConfig{
		id:   strategies.StrategyTokenBucket,
		caps: strategies.CapPrimary,
	}

	// missing fields
	err = (&Config{}).Validate()
	require.Error(t, err, "expected error for empty config")

	// missing storage
	cfg = Config{BaseKey: "base"}
	err = cfg.Validate()
	require.Error(t, err, "expected error for missing storage")

	// missing primary config
	cfg = Config{BaseKey: "base", Storage: mb}
	err = cfg.Validate()
	require.Error(t, err, "expected error for missing primary config")

	// invalid primary config validate error
	cfg = Config{BaseKey: "base", Storage: mb, PrimaryConfig: mockStrategyConfig{valid: errors.New("bad")}}
	err = cfg.Validate()
	require.Error(t, err, "expected error for invalid primary config")

	// secondary present but lacks CapSecondary
	secBad := mockStrategyConfig{caps: strategies.CapPrimary}
	cfg = Config{BaseKey: "base", Storage: mb, PrimaryConfig: primary, SecondaryConfig: secBad}
	err = cfg.Validate()
	require.Error(t, err, "expected error for secondary without CapSecondary")

	// primary cannot have CapSecondary when secondary provided
	primWithSecondary := mockStrategyConfig{caps: strategies.CapPrimary | strategies.CapSecondary}
	secGood := mockStrategyConfig{caps: strategies.CapSecondary}
	cfg = Config{BaseKey: "base", Storage: mb, PrimaryConfig: primWithSecondary, SecondaryConfig: secGood}
	err = cfg.Validate()
	require.Error(t, err, "expected error for primary having CapSecondary when secondary provided")

	// validate base key
	cfg = Config{Storage: mb, PrimaryConfig: primary}
	err = cfg.Validate()
	require.Error(t, err, "expected error for missing base key")

	withBaseKey := WithBaseKey("base")(&cfg)
	require.NoError(t, withBaseKey, "unexpected error")

	// valid minimal config
	cfg = Config{BaseKey: "base", Storage: mb, PrimaryConfig: primary}
	err = cfg.Validate()
	require.NoError(t, err, "unexpected error")

	// validate max retries
	config := Config{BaseKey: "base", Storage: mb, PrimaryConfig: primary}
	validRetries := WithMaxRetries(10)(&config)
	require.NoError(t, validRetries, "unexpected error")

	invalidRetries := WithMaxRetries(-1)(&config)
	require.Error(t, invalidRetries, "expected error for invalid max retries")

	retriesTooHigh := WithMaxRetries(math.MaxInt)(&config)
	require.Error(t, retriesTooHigh, "expected error for max retries too high")
}

func TestAllowAndResultFlow_SingleStrategy(t *testing.T) {
	// Prepare limiter with mocks; ensure key building works
	mb := &mockBackendOne{}
	primCfg := mockStrategyConfig{id: strategies.StrategyTokenBucket, caps: strategies.CapPrimary}

	ms := &mockStrategyOne{allowRes: strategies.Results{
		"q1": {Allowed: true},
		"q2": {Allowed: true},
	}}

	rl, err := newRateLimiter(Config{BaseKey: "base", Storage: mb, PrimaryConfig: primCfg})
	require.NoError(t, err, "RateLimiter should be created")
	rl.strategy = ms

	// request without explicit result
	user := "user"
	allowed, err := rl.Allow(context.Background(), AccessOptions{Key: user})
	require.NoError(t, err, "expected allowed without error")
	require.True(t, allowed, "expected allowed to be true")

	// Verify key composed and passed via WithKey to strategy config
	if c, ok := ms.lastConfig.(mockStrategyConfig); ok {
		assert.Equal(t, "base:user", c.key, "expected composed key 'base:user'")
	} else {
		t.Fatalf("strategy config type mismatch")
	}

	// request with result map capture
	resHolder := strategies.Results{}
	allowed, err = rl.Allow(context.Background(), AccessOptions{Result: &resHolder})
	require.NoError(t, err, "expected allowed with result")
	require.True(t, allowed, "expected allowed to be true")
	assert.True(t, reflect.DeepEqual(resHolder, ms.allowRes), "results not propagated correctly")
}

func TestPeekAndReset_DualStrategy_PassesCompositeConfig(t *testing.T) {
	// In dual strategy mode, the RateLimiter constructs strategies.CompositeConfig and delegates
	primCfg := mockStrategyConfig{caps: strategies.CapPrimary}
	secCfg := mockStrategyConfig{caps: strategies.CapSecondary}

	ms := &mockStrategyOne{getRes: strategies.Results{"p": {Allowed: true}}}

	rl := &RateLimiter{
		config:   Config{BaseKey: "base", Storage: &mockBackendOne{}, PrimaryConfig: primCfg, SecondaryConfig: secCfg},
		strategy: ms,
	}

	// Peek should forward CompositeConfig to strategy
	_, err := rl.Peek(context.Background(), AccessOptions{})
	require.NoError(t, err, "Peek error: %v", err)

	// Last config should be CompositeConfig with composite key set after WithKey
	// In the new atomic design, primary and secondary configs don't get keys since they use singleKeyAdapter
	if cc, ok := ms.lastConfig.(*composite.Config); ok {
		// Apply key so we can inspect composite key passed down via WithKey inside Peek
		cc = cc.WithKey("base:default").(*composite.Config)
		assert.NotEmpty(t, cc.CompositeKey(), "expected composite key after WithKey")
		assert.Equal(t, "base:base:default:c", cc.CompositeKey(), "composite key should match expected format")

		// Primary and secondary configs should not have keys in the new atomic design
		if pc, okp := cc.Primary.(mockStrategyConfig); okp {
			assert.Empty(t, pc.key, "primary config should not have key in atomic design")
		} else {
			t.Fatalf("primary strategy config type mismatch")
		}
		if sc, oks := cc.Secondary.(mockStrategyConfig); oks {
			assert.Empty(t, sc.key, "secondary config should not have key in atomic design")
		} else {
			t.Fatalf("secondary strategy config type mismatch")
		}
	} else {
		t.Fatalf("expected CompositeConfig, got %T", ms.lastConfig)
	}

	// Reset should also forward CompositeConfig
	require.NoError(t, rl.Reset(context.Background(), AccessOptions{}), "Reset error: %v", err)
}
