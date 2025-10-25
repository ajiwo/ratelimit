package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends"
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

func (m *mockBackendOne) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	m.setCalls++
	if m.data == nil {
		m.data = make(map[string]string)
	}
	m.data[key] = fmt.Sprintf("%v", value)
	return nil
}

func (m *mockBackendOne) CheckAndSet(ctx context.Context, key string, oldValue, newValue any, expiration time.Duration) (bool, error) {
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
	lastConfig strategies.StrategyConfig
	allowRes   map[string]strategies.Result
	allowErr   error
	getRes     map[string]strategies.Result
	getErr     error
	resetErr   error
}

func (m *mockStrategyOne) Allow(ctx context.Context, cfg strategies.StrategyConfig) (map[string]strategies.Result, error) {
	m.lastConfig = cfg
	return m.allowRes, m.allowErr
}

func (m *mockStrategyOne) GetResult(ctx context.Context, cfg strategies.StrategyConfig) (map[string]strategies.Result, error) {
	m.lastConfig = cfg
	return m.getRes, m.getErr
}

func (m *mockStrategyOne) Reset(ctx context.Context, cfg strategies.StrategyConfig) error {
	m.lastConfig = cfg
	return m.resetErr
}

// mockStrategyConfig implements strategies.StrategyConfig

type mockStrategyConfig struct {
	id    strategies.StrategyID
	caps  strategies.CapabilityFlags
	role  strategies.StrategyRole
	key   string
	valid error
}

func (m mockStrategyConfig) Validate() error                          { return m.valid }
func (m mockStrategyConfig) ID() strategies.StrategyID                { return m.id }
func (m mockStrategyConfig) Capabilities() strategies.CapabilityFlags { return m.caps }
func (m mockStrategyConfig) GetRole() strategies.StrategyRole         { return m.role }
func (m mockStrategyConfig) WithRole(role strategies.StrategyRole) strategies.StrategyConfig {
	m.role = role
	return m
}
func (m mockStrategyConfig) WithKey(key string) strategies.StrategyConfig {
	m.key = key
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
	cfg := &Config{}

	mb1 := &mockBackendOne{}
	require.NoError(t, WithBackend(mb1)(cfg), "unexpected error")
	assert.Equal(t, mb1, cfg.Storage, "backend not set")

	// swapping should close previous
	mb2 := &mockBackendOne{}
	require.NoError(t, WithBackend(mb2)(cfg), "unexpected error")
	assert.True(t, mb1.closed, "expected previous backend closed")
	assert.Equal(t, mb2, cfg.Storage, "backend not swapped")

	// nil backend should error
	require.Error(t, WithBackend(nil)(cfg), "expected error for nil backend")

	// closing error propagates
	cfg = &Config{Storage: &mockBackendOne{closeErr: errors.New("boom")}}
	require.Error(t, WithBackend(&mockBackendOne{})(cfg), "expected error when closing existing backend fails")
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
	mb := &mockBackendOne{}
	primary := mockStrategyConfig{id: strategies.StrategyTokenBucket, caps: strategies.CapPrimary}

	// missing fields
	require.Error(t, (Config{}).Validate(), "expected error for empty config")

	// missing storage
	require.Error(t, (Config{BaseKey: "base"}).Validate(), "expected error for missing storage")

	// missing primary config
	require.Error(t, (Config{BaseKey: "base", Storage: mb}).Validate(), "expected error for missing primary config")

	// invalid primary config validate error
	require.Error(t, (Config{BaseKey: "base", Storage: mb, PrimaryConfig: mockStrategyConfig{valid: errors.New("bad")}}).Validate(), "expected error for invalid primary config")

	// secondary present but lacks CapSecondary
	secBad := mockStrategyConfig{caps: strategies.CapPrimary}
	require.Error(t, (Config{BaseKey: "base", Storage: mb, PrimaryConfig: primary, SecondaryConfig: secBad}).Validate(), "expected error for secondary without CapSecondary")

	// primary cannot have CapSecondary when secondary provided
	primWithSecondary := mockStrategyConfig{caps: strategies.CapPrimary | strategies.CapSecondary}
	secGood := mockStrategyConfig{caps: strategies.CapSecondary}
	require.Error(t, (Config{BaseKey: "base", Storage: mb, PrimaryConfig: primWithSecondary, SecondaryConfig: secGood}).Validate(), "expected error for primary having CapSecondary when secondary provided")

	// valid minimal config
	require.NoError(t, (Config{BaseKey: "base", Storage: mb, PrimaryConfig: primary}).Validate(), "unexpected error")
}

func TestAllowAndResultFlow_SingleStrategy(t *testing.T) {
	// Prepare limiter with mocks; ensure key building works
	mb := &mockBackendOne{}
	primCfg := mockStrategyConfig{id: strategies.StrategyTokenBucket, caps: strategies.CapPrimary}

	ms := &mockStrategyOne{allowRes: map[string]strategies.Result{
		"q1": {Allowed: true},
		"q2": {Allowed: true},
	}}

	rl := &RateLimiter{
		config:          Config{BaseKey: "base", Storage: mb, PrimaryConfig: primCfg},
		primaryStrategy: ms,
	}

	// request without explicit result
	user := "user"
	allowed, err := rl.Allow(context.Background(), AccessOptions{Key: user})
	require.NoError(t, err, "expected allowed without error")
	require.True(t, allowed, "expected allowed to be true")

	// Verify key composed and passed via WithKey to strategy config
	if c, ok := ms.lastConfig.(mockStrategyConfig); ok {
		assert.Equal(t, "base:user", c.key, "expected composed key 'base:user'")
		assert.Equal(t, strategies.RolePrimary, c.role, "expected primary role")
	} else {
		t.Fatalf("strategy config type mismatch")
	}

	// request with result map capture
	resHolder := map[string]strategies.Result{}
	allowed, err = rl.Allow(context.Background(), AccessOptions{Result: &resHolder})
	require.NoError(t, err, "expected allowed with result")
	require.True(t, allowed, "expected allowed to be true")
	assert.True(t, reflect.DeepEqual(resHolder, ms.allowRes), "results not propagated correctly")
}

func TestPeekAndReset_DualStrategy_PassesCompositeConfig(t *testing.T) {
	// In dual strategy mode, the RateLimiter constructs strategies.CompositeConfig and delegates
	primCfg := mockStrategyConfig{caps: strategies.CapPrimary}
	secCfg := mockStrategyConfig{caps: strategies.CapSecondary}

	ms := &mockStrategyOne{getRes: map[string]strategies.Result{"p": {Allowed: true}}}

	rl := &RateLimiter{
		config:          Config{BaseKey: "base", Storage: &mockBackendOne{}, PrimaryConfig: primCfg, SecondaryConfig: secCfg},
		primaryStrategy: ms,
	}

	// Peek should forward CompositeConfig to strategy
	_, err := rl.Peek(context.Background(), AccessOptions{})
	require.NoError(t, err, "Peek error: %v", err)

	// Last config should be CompositeConfig with both configs containing same fully qualified key after WithKey
	if cc, ok := ms.lastConfig.(strategies.CompositeConfig); ok {
		// Apply key so we can inspect keys passed down via WithKey inside Peek
		cc = cc.WithKey("base:default").(strategies.CompositeConfig)
		if pc, okp := cc.Primary.(mockStrategyConfig); okp {
			assert.NotEmpty(t, pc.key, "expected key on primary after WithKey")
		} else {
			t.Fatalf("primary strategy config type mismatch")
		}
		if sc, oks := cc.Secondary.(mockStrategyConfig); oks {
			assert.NotEmpty(t, sc.key, "expected key on secondary after WithKey")
		} else {
			t.Fatalf("secondary strategy config type mismatch")
		}
	} else {
		t.Fatalf("expected CompositeConfig, got %T", ms.lastConfig)
	}

	// Reset should also forward CompositeConfig
	require.NoError(t, rl.Reset(context.Background(), AccessOptions{}), "Reset error: %v", err)
}
