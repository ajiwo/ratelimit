package composite

import (
	"context"
	"errors"
	"reflect"
	"testing"

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
	role       strategies.Role
	key        string
	valid      error
	maxRetries int
}

func (m compMockConfig) Validate() error                          { return m.valid }
func (m compMockConfig) ID() strategies.ID                        { return m.id }
func (m compMockConfig) Capabilities() strategies.CapabilityFlags { return m.caps }
func (m compMockConfig) GetRole() strategies.Role                 { return m.role }
func (m compMockConfig) WithRole(role strategies.Role) strategies.Config {
	m.role = role
	return m
}
func (m compMockConfig) WithKey(key string) strategies.Config { m.key = key; return m }
func (m compMockConfig) MaxRetries() int                      { return m.maxRetries }
func (m compMockConfig) WithMaxRetries(retries int) strategies.Config {
	m.maxRetries = retries
	return m
}

func TestCompositeConfigValidate(t *testing.T) {
	pri := compMockConfig{caps: strategies.CapPrimary}
	sec := compMockConfig{caps: strategies.CapSecondary}

	t.Run("baseKey and Presence", func(t *testing.T) {
		err := (Config{}).Validate()
		require.Error(t, err, "expected error for empty base key and nil strategies")
		err = (Config{BaseKey: "k", Secondary: sec}).Validate()
		require.Error(t, err, "expected error when primary is nil")

		err = (Config{BaseKey: "k", Primary: pri}).Validate()
		require.Error(t, err, "expected error when secondary is nil")

		err = (Config{BaseKey: "k", Primary: pri, Secondary: sec}).Validate()
		require.NoError(t, err, "unexpected: %v", err)
	})

	t.Run("inner validate", func(t *testing.T) {
		bad := compMockConfig{caps: strategies.CapPrimary, valid: errors.New("bad")}
		err := (Config{BaseKey: "k", Primary: bad, Secondary: sec}).Validate()
		require.Error(t, err, "expected error from primary validate")
	})

	t.Run("capabilities", func(t *testing.T) {
		noPriCap := compMockConfig{}
		err := (Config{BaseKey: "k", Primary: noPriCap, Secondary: sec}).Validate()
		require.Error(t, err, "expected error for primary missing CapPrimary")

		noSecCap := compMockConfig{caps: strategies.CapPrimary}
		err = (Config{BaseKey: "k", Primary: pri, Secondary: noSecCap}).Validate()
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
	if cc.GetRole() != strategies.RolePrimary {
		t.Fatalf("GetRole should be primary")
	}
	if got := cc.WithRole(strategies.RoleSecondary).(Config); !reflect.DeepEqual(got, cc) {
		t.Fatalf("WithRole should return same config")
	}
}

func TestCompositeConfigHelpers_WithKey(t *testing.T) {
	pri := compMockConfig{caps: strategies.CapPrimary}
	sec := compMockConfig{caps: strategies.CapSecondary}
	cc := Config{BaseKey: "k", Primary: pri, Secondary: sec}
	applied := cc.WithKey("fully:qualified").(Config)
	if p, ok := applied.Primary.(compMockConfig); !ok || p.key != "k:fully:qualified:p" {
		t.Fatalf("primary key not applied")
	}
	if s, ok := applied.Secondary.(compMockConfig); !ok || s.key != "k:fully:qualified:s" {
		t.Fatalf("secondary key not applied")
	}
}

func TestCompositeStrategyFlows(t *testing.T) {
	var storage backends.Backend = nil

	// Create mock strategies
	pri := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: true}}, allowRes: strategies.Results{"p": {Allowed: true}}}
	sec := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: true}}, allowRes: strategies.Results{"s": {Allowed: true}}}

	// Register mock strategies
	strategies.Register(strategies.ID(1), func(_ backends.Backend) strategies.Strategy { return pri })
	strategies.Register(strategies.ID(2), func(_ backends.Backend) strategies.Strategy { return sec })

	// Create configs with registered IDs
	priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
	secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

	// Create composite strategy with new signature
	comp, err := NewComposite(storage, priConfig, secConfig)
	require.NoError(t, err, "Failed to create composite strategy")

	cfg := Config{BaseKey: "k", Primary: priConfig, Secondary: secConfig}

	// GetResult aggregates results with prefixes
	res, err := comp.Peek(context.Background(), cfg)
	require.NoError(t, err, "GetResult error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary_p in results")
	require.Contains(t, res, "secondary_s", "expected secondary_s in results")

	// Allow path where both allow
	_, err = comp.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)

	// Test primary denial - create new composite with denying primary strategy
	primDeny := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: false}}}
	strategies.Register(strategies.ID(3), func(_ backends.Backend) strategies.Strategy { return primDeny })
	primDenyConfig := compMockConfig{id: strategies.ID(3), caps: strategies.CapPrimary}

	compDeny, err := NewComposite(storage, primDenyConfig, secConfig)
	require.NoError(t, err, "Failed to create composite with denying primary")

	res, err = compDeny.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary results present on denial")

	// Test secondary denial - create new composite with denying secondary strategy
	secDeny := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: false}}}
	strategies.Register(strategies.ID(4), func(_ backends.Backend) strategies.Strategy { return secDeny })
	secDenyConfig := compMockConfig{id: strategies.ID(4), caps: strategies.CapSecondary}

	compSecDeny, err := NewComposite(storage, priConfig, secDenyConfig)
	require.NoError(t, err, "Failed to create composite with denying secondary")

	res, err = compSecDeny.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "secondary_s", "expected secondary results present on denial")

	// Test primary error path
	priErr := &compMockStrategy{getErr: errors.New("bad primary")}
	strategies.Register(strategies.ID(5), func(_ backends.Backend) strategies.Strategy { return priErr })
	priErrConfig := compMockConfig{id: strategies.ID(5), caps: strategies.CapPrimary}

	compPriErr, err := NewComposite(storage, priErrConfig, secConfig)
	require.NoError(t, err, "Failed to create composite with erroring primary")

	_, err = compPriErr.Allow(context.Background(), cfg)
	require.Error(t, err, "expected primary get error")

	// Test secondary error path
	secErr := &compMockStrategy{getErr: errors.New("bad secondary")}
	strategies.Register(strategies.ID(6), func(_ backends.Backend) strategies.Strategy { return secErr })
	secErrConfig := compMockConfig{id: strategies.ID(6), caps: strategies.CapSecondary}

	compSecErr, err := NewComposite(storage, priConfig, secErrConfig)
	require.NoError(t, err, "Failed to create composite with erroring secondary")

	_, err = compSecErr.Allow(context.Background(), cfg)
	require.Error(t, err, "expected secondary get error")

	// Test reset error path
	resetErr := &compMockStrategy{resetErr: errors.New("reset fail")}
	strategies.Register(strategies.ID(7), func(_ backends.Backend) strategies.Strategy { return resetErr })
	resetErrConfig := compMockConfig{id: strategies.ID(7), caps: strategies.CapSecondary}

	compResetErr, err := NewComposite(storage, priConfig, resetErrConfig)
	require.NoError(t, err, "Failed to create composite with reset error")

	err = compResetErr.Reset(context.Background(), cfg)
	require.Error(t, err, "expected reset error")
}

func TestNewCompositeErrorScenarios(t *testing.T) {
	var storage backends.Backend = nil

	t.Run("nil primary config", func(t *testing.T) {
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}
		_, err := NewComposite(storage, nil, secConfig)
		require.Error(t, err, "expected error for nil primary config")
		require.Contains(t, err.Error(), "primary strategy config cannot be nil")
	})

	t.Run("nil secondary config", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		_, err := NewComposite(storage, priConfig, nil)
		require.Error(t, err, "expected error for nil secondary config")
		require.Contains(t, err.Error(), "secondary strategy config cannot be nil")
	})

	t.Run("primary config validation error", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary, valid: errors.New("primary validation failed")}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for primary validation failure")
		require.Contains(t, err.Error(), "primary config validation failed")
	})

	t.Run("secondary config validation error", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary, valid: errors.New("secondary validation failed")}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for secondary validation failure")
		require.Contains(t, err.Error(), "secondary config validation failed")
	})

	t.Run("primary missing CapPrimary capability", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapSecondary} // Wrong capability
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for primary missing CapPrimary")
		require.Contains(t, err.Error(), "primary strategy must support primary capability")
	})

	t.Run("secondary missing CapSecondary capability", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapPrimary} // Wrong capability

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for secondary missing CapSecondary")
		require.Contains(t, err.Error(), "secondary strategy must support secondary capability")
	})

	t.Run("unregistered primary strategy ID", func(t *testing.T) {
		priConfig := compMockConfig{id: strategies.ID(200), caps: strategies.CapPrimary} // Unregistered ID
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for unregistered primary strategy")
		require.Contains(t, err.Error(), "failed to create primary strategy")
	})

	t.Run("unregistered secondary strategy ID", func(t *testing.T) {
		// Register a valid primary strategy
		priStrategy := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: true}}}
		strategies.Register(strategies.ID(1), func(_ backends.Backend) strategies.Strategy { return priStrategy })

		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		secConfig := compMockConfig{id: strategies.ID(201), caps: strategies.CapSecondary} // Unregistered ID

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for unregistered secondary strategy")
		require.Contains(t, err.Error(), "failed to create secondary strategy")
	})

	t.Run("successful creation", func(t *testing.T) {
		// Register valid strategies
		priStrategy := &compMockStrategy{getRes: strategies.Results{"p": {Allowed: true}}}
		secStrategy := &compMockStrategy{getRes: strategies.Results{"s": {Allowed: true}}}
		strategies.Register(strategies.ID(1), func(_ backends.Backend) strategies.Strategy { return priStrategy })
		strategies.Register(strategies.ID(2), func(_ backends.Backend) strategies.Strategy { return secStrategy })

		priConfig := compMockConfig{id: strategies.ID(1), caps: strategies.CapPrimary}
		secConfig := compMockConfig{id: strategies.ID(2), caps: strategies.CapSecondary}

		composite, err := NewComposite(storage, priConfig, secConfig)
		require.NoError(t, err, "expected successful composite creation")
		require.NotNil(t, composite, "composite should not be nil")
	})
}
