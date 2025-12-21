package composite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/require"
)

// mockStrategy for composite tests

type compMockStrategy struct {
	allowRes strategies.Results
	allowErr error
	getRes   strategies.Results
	getErr   error
	resetErr error
}

func (m *compMockStrategy) Allow(ctx context.Context, cfg strategies.Config) (strategies.Results, error) {
	return m.allowRes, m.allowErr
}

func (m *compMockStrategy) Peek(ctx context.Context, cfg strategies.Config) (strategies.Results, error) {
	return m.getRes, m.getErr
}
func (m *compMockStrategy) Reset(ctx context.Context, cfg strategies.Config) error {
	return m.resetErr
}

// mock config implementing strategies.StrategyConfig

type compMockConfig struct {
	id         strategies.ID
	caps       strategies.CapabilityFlags
	key        string
	valid      error
	maxRetries int
}

func (m compMockConfig) Validate() error                          { return m.valid }
func (m compMockConfig) ID() strategies.ID                        { return m.id }
func (m compMockConfig) Capabilities() strategies.CapabilityFlags { return m.caps }

func (m compMockConfig) WithKey(key string) strategies.Config { m.key = key; return m }
func (m compMockConfig) GetMaxRetries() int                   { return m.maxRetries + 1 }
func (m compMockConfig) WithMaxRetries(retries int) strategies.Config {
	m.maxRetries = retries
	return m
}

func TestCompositeConfigValidate(t *testing.T) {
	pri := compMockConfig{caps: strategies.CapPrimary}
	sec := compMockConfig{caps: strategies.CapSecondary}

	t.Run("baseKey and Presence", func(t *testing.T) {
		err := (&Config{}).Validate()
		require.Error(t, err, "expected error for empty base key and nil strategies")
		err = (&Config{BaseKey: "k", Secondary: sec}).Validate()
		require.Error(t, err, "expected error when primary is nil")

		err = (&Config{BaseKey: "k", Primary: pri}).Validate()
		require.Error(t, err, "expected error when secondary is nil")

		err = (&Config{BaseKey: "k", Primary: pri, Secondary: sec}).Validate()
		require.NoError(t, err, "unexpected: %v", err)
	})

	t.Run("inner validate", func(t *testing.T) {
		bad := compMockConfig{caps: strategies.CapPrimary, valid: errors.New("bad")}
		err := (&Config{BaseKey: "k", Primary: bad, Secondary: sec}).Validate()
		require.Error(t, err, "expected error from primary validate")
	})

	t.Run("capabilities", func(t *testing.T) {
		noPriCap := compMockConfig{}
		err := (&Config{BaseKey: "k", Primary: noPriCap, Secondary: sec}).Validate()
		require.Error(t, err, "expected error for primary missing CapPrimary")

		noSecCap := compMockConfig{caps: strategies.CapPrimary}
		err = (&Config{BaseKey: "k", Primary: pri, Secondary: noSecCap}).Validate()
		require.Error(t, err, "expected error for secondary missing CapSecondary")
	})
}

func TestCompositeConfigHelpers_IDCapsRole(t *testing.T) {
	pri := compMockConfig{caps: strategies.CapPrimary}
	sec := compMockConfig{caps: strategies.CapSecondary}
	cc := Config{BaseKey: "k", Primary: pri, Secondary: sec}
	if cc.ID() != strategies.StrategyComposite {
		t.Fatalf("ID mismatch")
	}
	if cc.Capabilities() != (strategies.CapPrimary | strategies.CapSecondary) {
		t.Fatalf("capabilities mismatch")
	}

}

func TestCompositeConfigHelpers_WithKey(t *testing.T) {
	pri := compMockConfig{caps: strategies.CapPrimary}
	sec := compMockConfig{caps: strategies.CapSecondary}
	cc := Config{BaseKey: "k", Primary: pri, Secondary: sec}
	applied := cc.WithKey("fully:qualified").(*Config)
	if applied.CompositeKey() != "k:fully:qualified:c" {
		t.Fatalf("composite key not applied correctly, got: %s", applied.CompositeKey())
	}
}

func TestCompositeStrategyFlows(t *testing.T) {
	// Create a mock backend that supports CAS operations
	storage := &mockBackend{
		data: make(map[string]mockData),
	}

	// Create mock strategies that will be used by the composite
	pri := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: true}}, allowRes: strategies.Results{"p": {Allowed: true}}}
	sec := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: true}}, allowRes: strategies.Results{"s": {Allowed: true}}}

	// Register mock strategies
	strategies.Register(strategies.ID(1), func(_ backends.Backend) strategies.Strategy { return pri })
	strategies.Register(strategies.ID(2), func(_ backends.Backend) strategies.Strategy { return sec })

	// Create configs with registered IDs
	priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
	secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

	// Create composite strategy
	comp, err := New(storage, priConfig, secConfig)
	require.NoError(t, err, "Failed to create composite strategy")

	cfg := &Config{BaseKey: "k", Primary: priConfig, Secondary: secConfig}
	cfg = cfg.WithKey("test").(*Config)

	// Test Peek - should work even with no state
	res, err := comp.Peek(t.Context(), cfg)
	require.NoError(t, err, "Peek error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary_p in results")
	require.Contains(t, res, "secondary_s", "expected secondary_s in results")

	// Test Allow path where both allow
	res, err = comp.Allow(t.Context(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary_p in results")
	require.Contains(t, res, "secondary_s", "expected secondary_s in results")
	require.True(t, res["primary_p"].Allowed, "primary should allow")
	require.True(t, res["secondary_s"].Allowed, "secondary should allow")

	// Test primary denial - create denying primary strategy
	primDeny := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: false}}}
	strategies.Register(strategies.ID(3), func(_ backends.Backend) strategies.Strategy { return primDeny })
	primDenyConfig := compMockConfig{id: strategies.ID(3), caps: strategies.CapPrimary}

	compDeny, err := New(storage, primDenyConfig, secConfig)
	require.NoError(t, err, "Failed to create composite with denying primary")

	cfgDeny := &Config{BaseKey: "k", Primary: primDenyConfig, Secondary: secConfig}
	cfgDeny = cfgDeny.WithKey("deny").(*Config)

	res, err = compDeny.Allow(t.Context(), cfgDeny)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary results present on denial")
	require.False(t, res["primary_p"].Allowed, "primary should deny")

	// Test secondary denial - create denying secondary strategy
	secDeny := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: false}}}
	strategies.Register(strategies.ID(4), func(_ backends.Backend) strategies.Strategy { return secDeny })
	secDenyConfig := compMockConfig{id: strategies.ID(4), caps: strategies.CapSecondary}

	compSecDeny, err := New(storage, priConfig, secDenyConfig)
	require.NoError(t, err, "Failed to create composite with denying secondary")

	cfgSecDeny := &Config{BaseKey: "k", Primary: priConfig, Secondary: secDenyConfig}
	cfgSecDeny = cfgSecDeny.WithKey("secdeny").(*Config)

	res, err = compSecDeny.Allow(t.Context(), cfgSecDeny)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "secondary_s", "expected secondary results present on denial")
	require.False(t, res["secondary_s"].Allowed, "secondary should deny")

	// Test reset functionality
	err = comp.Reset(t.Context(), cfg)
	require.NoError(t, err, "Reset error: %v", err)
}

// mockBackend implements backends.Backend for testing
type mockBackend struct {
	data map[string]mockData
}

type mockData struct {
	value      string
	expiration time.Duration
}

func (m *mockBackend) Get(ctx context.Context, key string) (string, error) {
	if data, exists := m.data[key]; exists {
		return data.value, nil
	}
	return "", nil
}

func (m *mockBackend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	m.data[key] = mockData{value: value, expiration: expiration}
	return nil
}

func (m *mockBackend) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	current, exists := m.data[key]
	if !exists && oldValue != "" {
		return false, nil
	}
	if exists && current.value != oldValue {
		return false, nil
	}
	m.data[key] = mockData{value: newValue, expiration: expiration}
	return true, nil
}

func (m *mockBackend) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockBackend) Close() error {
	return nil
}

func TestNewCompositeErrorScenarios(t *testing.T) {
	var storage backends.Backend = nil

	t.Run("nil primary config", func(t *testing.T) {
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}
		_, err := New(storage, nil, secConfig)
		require.Error(t, err, "expected error for nil primary config")
		require.Contains(t, err.Error(), "primary strategy config cannot be nil")
	})

	t.Run("nil secondary config", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		_, err := New(storage, priConfig, nil)
		require.Error(t, err, "expected error for nil secondary config")
		require.Contains(t, err.Error(), "secondary strategy config cannot be nil")
	})

	t.Run("primary config validation error", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary, valid: errors.New("primary validation failed")}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

		_, err := New(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for primary validation failure")
		require.Contains(t, err.Error(), "primary config validation failed")
	})

	t.Run("secondary config validation error", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary, valid: errors.New("secondary validation failed")}

		_, err := New(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for secondary validation failure")
		require.Contains(t, err.Error(), "secondary config validation failed")
	})

	t.Run("primary missing CapPrimary capability", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapSecondary} // Wrong capability
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

		_, err := New(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for primary missing CapPrimary")
		require.Contains(t, err.Error(), "primary strategy must support primary capability")
	})

	t.Run("secondary missing CapSecondary capability", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapPrimary} // Wrong capability

		_, err := New(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for secondary missing CapSecondary")
		require.Contains(t, err.Error(), "secondary strategy must support secondary capability")
	})

	t.Run("successful creation", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

		composite, err := New(storage, priConfig, secConfig)
		require.NoError(t, err, "expected successful composite creation")
		require.NotNil(t, composite, "composite should not be nil")
	})
}

func TestCompositeAtomicBehavior(t *testing.T) {
	storage := &mockBackend{
		data: make(map[string]mockData),
	}

	// Create strategies where primary allows but secondary denies
	pri := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: true}}, allowRes: strategies.Results{"p": {Allowed: true}}}
	sec := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: false}}}

	strategies.Register(strategies.ID(10), func(_ backends.Backend) strategies.Strategy { return pri })
	strategies.Register(strategies.ID(11), func(_ backends.Backend) strategies.Strategy { return sec })

	priConfig := compMockConfig{id: strategies.ID(10), caps: strategies.CapPrimary}
	secConfig := compMockConfig{id: strategies.ID(11), caps: strategies.CapSecondary}

	comp, err := New(storage, priConfig, secConfig)
	require.NoError(t, err, "Failed to create composite strategy")

	cfg := &Config{BaseKey: "k", Primary: priConfig, Secondary: secConfig}
	cfg = cfg.WithKey("atomic").(*Config)

	// Test that no state is committed when secondary denies
	res, err := comp.Allow(t.Context(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.False(t, res["secondary_s"].Allowed, "secondary should deny")

	// Verify no composite state was stored
	compositeValue, err := storage.Get(t.Context(), cfg.CompositeKey())
	require.NoError(t, err, "Failed to get composite state")
	require.Empty(t, compositeValue, "No state should be stored when secondary denies")
}

func TestCompositeCASRetry(t *testing.T) {
	storage := &failingCASBackend{
		data:       make(map[string]mockData),
		failCount:  2, // Fail first 2 CAS attempts
		maxRetries: 5,
	}

	pri := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: true}}, allowRes: strategies.Results{"p": {Allowed: true}}}
	sec := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: true}}, allowRes: strategies.Results{"s": {Allowed: true}}}

	strategies.Register(strategies.ID(20), func(_ backends.Backend) strategies.Strategy { return pri })
	strategies.Register(strategies.ID(21), func(_ backends.Backend) strategies.Strategy { return sec })

	priConfig := compMockConfig{id: strategies.ID(20), caps: strategies.CapPrimary}
	secConfig := compMockConfig{id: strategies.ID(21), caps: strategies.CapSecondary}

	comp, err := New(storage, priConfig, secConfig)
	require.NoError(t, err, "Failed to create composite strategy")

	cfg := &Config{BaseKey: "k", Primary: priConfig, Secondary: secConfig}
	cfg = cfg.WithKey("retry").WithMaxRetries(3).(*Config)

	// Should succeed after retries
	res, err := comp.Allow(t.Context(), cfg)
	require.NoError(t, err, "Allow should succeed after retries")
	require.True(t, res["primary_p"].Allowed, "primary should allow")
	require.True(t, res["secondary_s"].Allowed, "secondary should allow")
}

func TestCompositeBackwardCompatibility(t *testing.T) {
	storage := &mockBackend{
		data: make(map[string]mockData),
	}

	pri := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: true}}, allowRes: strategies.Results{"p": {Allowed: true}}}
	sec := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: true}}, allowRes: strategies.Results{"s": {Allowed: true}}}

	strategies.Register(strategies.ID(30), func(_ backends.Backend) strategies.Strategy { return pri })
	strategies.Register(strategies.ID(31), func(_ backends.Backend) strategies.Strategy { return sec })

	priConfig := compMockConfig{id: strategies.ID(30), caps: strategies.CapPrimary}
	secConfig := compMockConfig{id: strategies.ID(31), caps: strategies.CapSecondary}

	comp, err := New(storage, priConfig, secConfig)
	require.NoError(t, err, "Failed to create composite strategy")

	cfg := &Config{BaseKey: "k", Primary: priConfig, Secondary: secConfig}
	cfg = cfg.WithKey("compat").(*Config)

	// Test with missing/invalid composite state (should treat as empty)
	storage.data[cfg.CompositeKey()] = mockData{value: "invalid_format", expiration: 0}

	res, err := comp.Peek(t.Context(), cfg)
	require.NoError(t, err, "Peek should handle invalid state gracefully")
	require.Contains(t, res, "primary_p", "expected primary_p in results")
	require.Contains(t, res, "secondary_s", "expected secondary_s in results")
}

// failingCASBackend is a mock backend that fails CAS operations a specified number of times
type failingCASBackend struct {
	data       map[string]mockData
	failCount  int
	attempts   int
	maxRetries int
}

func (f *failingCASBackend) Get(ctx context.Context, key string) (string, error) {
	if data, exists := f.data[key]; exists {
		return data.value, nil
	}
	return "", nil
}

func (f *failingCASBackend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	f.data[key] = mockData{value: value, expiration: expiration}
	return nil
}

func (f *failingCASBackend) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	f.attempts++
	if f.attempts <= f.failCount {
		return false, nil // Simulate CAS failure
	}

	current, exists := f.data[key]
	if !exists && oldValue != "" {
		return false, nil
	}
	if exists && current.value != oldValue {
		return false, nil
	}
	f.data[key] = mockData{value: newValue, expiration: expiration}
	return true, nil
}

func (f *failingCASBackend) Delete(ctx context.Context, key string) error {
	delete(f.data, key)
	return nil
}

func (f *failingCASBackend) Close() error {
	return nil
}
