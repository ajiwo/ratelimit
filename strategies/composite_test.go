package strategies

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/stretchr/testify/require"
)

// mockStrategy for composite tests

type compMockStrategy struct {
	allowRes Results
	allowErr error
	getRes   Results
	getErr   error
	resetErr error
}

func (m *compMockStrategy) Allow(ctx context.Context, cfg StrategyConfig) (Results, error) {
	return m.allowRes, m.allowErr
}

func (m *compMockStrategy) Peek(ctx context.Context, cfg StrategyConfig) (Results, error) {
	return m.getRes, m.getErr
}
func (m *compMockStrategy) Reset(ctx context.Context, cfg StrategyConfig) error { return m.resetErr }

// mock config implementing StrategyConfig

type compMockConfig struct {
	id         StrategyID
	caps       CapabilityFlags
	role       StrategyRole
	key        string
	valid      error
	maxRetries int
}

func (m compMockConfig) Validate() error                           { return m.valid }
func (m compMockConfig) ID() StrategyID                            { return m.id }
func (m compMockConfig) Capabilities() CapabilityFlags             { return m.caps }
func (m compMockConfig) GetRole() StrategyRole                     { return m.role }
func (m compMockConfig) WithRole(role StrategyRole) StrategyConfig { m.role = role; return m }
func (m compMockConfig) WithKey(key string) StrategyConfig         { m.key = key; return m }
func (m compMockConfig) MaxRetries() int                           { return m.maxRetries }
func (m compMockConfig) WithMaxRetries(retries int) StrategyConfig {
	m.maxRetries = retries
	return m
}

func TestCompositeConfigValidate(t *testing.T) {
	pri := compMockConfig{caps: CapPrimary}
	sec := compMockConfig{caps: CapSecondary}

	t.Run("baseKey and Presence", func(t *testing.T) {
		err := (CompositeConfig{}).Validate()
		require.Error(t, err, "expected error for empty base key and nil strategies")
		err = (CompositeConfig{BaseKey: "k", Secondary: sec}).Validate()
		require.Error(t, err, "expected error when primary is nil")

		err = (CompositeConfig{BaseKey: "k", Primary: pri}).Validate()
		require.Error(t, err, "expected error when secondary is nil")

		err = (CompositeConfig{BaseKey: "k", Primary: pri, Secondary: sec}).Validate()
		require.NoError(t, err, "unexpected: %v", err)
	})

	t.Run("inner validate", func(t *testing.T) {
		bad := compMockConfig{caps: CapPrimary, valid: errors.New("bad")}
		err := (CompositeConfig{BaseKey: "k", Primary: bad, Secondary: sec}).Validate()
		require.Error(t, err, "expected error from primary validate")
	})

	t.Run("capabilities", func(t *testing.T) {
		noPriCap := compMockConfig{}
		err := (CompositeConfig{BaseKey: "k", Primary: noPriCap, Secondary: sec}).Validate()
		require.Error(t, err, "expected error for primary missing CapPrimary")

		noSecCap := compMockConfig{caps: CapPrimary}
		err = (CompositeConfig{BaseKey: "k", Primary: pri, Secondary: noSecCap}).Validate()
		require.Error(t, err, "expected error for secondary missing CapSecondary")
	})
}

func TestCompositeConfigHelpers_IDCapsRole(t *testing.T) {
	pri := compMockConfig{caps: CapPrimary}
	sec := compMockConfig{caps: CapSecondary}
	cc := CompositeConfig{BaseKey: "k", Primary: pri, Secondary: sec}
	if cc.ID() != StrategyComposite {
		t.Fatalf("ID mismatch")
	}
	if cc.Capabilities() != (CapPrimary | CapSecondary) {
		t.Fatalf("capabilities mismatch")
	}
	if cc.GetRole() != RolePrimary {
		t.Fatalf("GetRole should be primary")
	}
	if got := cc.WithRole(RoleSecondary).(CompositeConfig); !reflect.DeepEqual(got, cc) {
		t.Fatalf("WithRole should return same config")
	}
}

func TestCompositeConfigHelpers_WithKey(t *testing.T) {
	pri := compMockConfig{caps: CapPrimary}
	sec := compMockConfig{caps: CapSecondary}
	cc := CompositeConfig{BaseKey: "k", Primary: pri, Secondary: sec}
	applied := cc.WithKey("fully:qualified").(CompositeConfig)
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
	pri := &compMockStrategy{getRes: Results{"p": {Allowed: true}}, allowRes: Results{"p": {Allowed: true}}}
	sec := &compMockStrategy{getRes: Results{"s": {Allowed: true}}, allowRes: Results{"s": {Allowed: true}}}

	// Register mock strategies
	Register(StrategyID(1), func(_ backends.Backend) Strategy { return pri })
	Register(StrategyID(2), func(_ backends.Backend) Strategy { return sec })
	defer func() {
		// Clean up registry (in a real implementation, you'd have an Unregister function)
		registeredStrategies = make(map[StrategyID]StrategyFactory)
	}()

	// Create configs with registered IDs
	priConfig := compMockConfig{id: StrategyID(1), caps: CapPrimary}
	secConfig := compMockConfig{id: StrategyID(2), caps: CapSecondary}

	// Create composite strategy with new signature
	comp, err := NewComposite(storage, priConfig, secConfig)
	require.NoError(t, err, "Failed to create composite strategy")

	cfg := CompositeConfig{BaseKey: "k", Primary: priConfig, Secondary: secConfig}

	// GetResult aggregates results with prefixes
	res, err := comp.Peek(context.Background(), cfg)
	require.NoError(t, err, "GetResult error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary_p in results")
	require.Contains(t, res, "secondary_s", "expected secondary_s in results")

	// Allow path where both allow
	_, err = comp.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)

	// Test primary denial - create new composite with denying primary strategy
	primDeny := &compMockStrategy{getRes: Results{"p": {Allowed: false}}}
	Register(StrategyID(3), func(_ backends.Backend) Strategy { return primDeny })
	primDenyConfig := compMockConfig{id: StrategyID(3), caps: CapPrimary}

	compDeny, err := NewComposite(storage, primDenyConfig, secConfig)
	require.NoError(t, err, "Failed to create composite with denying primary")

	res, err = compDeny.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary results present on denial")

	// Test secondary denial - create new composite with denying secondary strategy
	secDeny := &compMockStrategy{getRes: Results{"s": {Allowed: false}}}
	Register(StrategyID(4), func(_ backends.Backend) Strategy { return secDeny })
	secDenyConfig := compMockConfig{id: StrategyID(4), caps: CapSecondary}

	compSecDeny, err := NewComposite(storage, priConfig, secDenyConfig)
	require.NoError(t, err, "Failed to create composite with denying secondary")

	res, err = compSecDeny.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "secondary_s", "expected secondary results present on denial")

	// Test primary error path
	priErr := &compMockStrategy{getErr: errors.New("bad primary")}
	Register(StrategyID(5), func(_ backends.Backend) Strategy { return priErr })
	priErrConfig := compMockConfig{id: StrategyID(5), caps: CapPrimary}

	compPriErr, err := NewComposite(storage, priErrConfig, secConfig)
	require.NoError(t, err, "Failed to create composite with erroring primary")

	_, err = compPriErr.Allow(context.Background(), cfg)
	require.Error(t, err, "expected primary get error")

	// Test secondary error path
	secErr := &compMockStrategy{getErr: errors.New("bad secondary")}
	Register(StrategyID(6), func(_ backends.Backend) Strategy { return secErr })
	secErrConfig := compMockConfig{id: StrategyID(6), caps: CapSecondary}

	compSecErr, err := NewComposite(storage, priConfig, secErrConfig)
	require.NoError(t, err, "Failed to create composite with erroring secondary")

	_, err = compSecErr.Allow(context.Background(), cfg)
	require.Error(t, err, "expected secondary get error")

	// Test reset error path
	resetErr := &compMockStrategy{resetErr: errors.New("reset fail")}
	Register(StrategyID(7), func(_ backends.Backend) Strategy { return resetErr })
	resetErrConfig := compMockConfig{id: StrategyID(7), caps: CapSecondary}

	compResetErr, err := NewComposite(storage, priConfig, resetErrConfig)
	require.NoError(t, err, "Failed to create composite with reset error")

	err = compResetErr.Reset(context.Background(), cfg)
	require.Error(t, err, "expected reset error")
}

func TestNewCompositeErrorScenarios(t *testing.T) {
	var storage backends.Backend = nil

	t.Run("nil primary config", func(t *testing.T) {
		secConfig := compMockConfig{id: StrategyID(2), caps: CapSecondary}
		_, err := NewComposite(storage, nil, secConfig)
		require.Error(t, err, "expected error for nil primary config")
		require.Contains(t, err.Error(), "primary strategy config cannot be nil")
	})

	t.Run("nil secondary config", func(t *testing.T) {
		priConfig := compMockConfig{id: StrategyID(1), caps: CapPrimary}
		_, err := NewComposite(storage, priConfig, nil)
		require.Error(t, err, "expected error for nil secondary config")
		require.Contains(t, err.Error(), "secondary strategy config cannot be nil")
	})

	t.Run("primary config validation error", func(t *testing.T) {
		priConfig := compMockConfig{id: StrategyID(1), caps: CapPrimary, valid: errors.New("primary validation failed")}
		secConfig := compMockConfig{id: StrategyID(2), caps: CapSecondary}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for primary validation failure")
		require.Contains(t, err.Error(), "primary config validation failed")
	})

	t.Run("secondary config validation error", func(t *testing.T) {
		priConfig := compMockConfig{id: StrategyID(1), caps: CapPrimary}
		secConfig := compMockConfig{id: StrategyID(2), caps: CapSecondary, valid: errors.New("secondary validation failed")}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for secondary validation failure")
		require.Contains(t, err.Error(), "secondary config validation failed")
	})

	t.Run("primary missing CapPrimary capability", func(t *testing.T) {
		priConfig := compMockConfig{id: StrategyID(1), caps: CapSecondary} // Wrong capability
		secConfig := compMockConfig{id: StrategyID(2), caps: CapSecondary}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for primary missing CapPrimary")
		require.Contains(t, err.Error(), "primary strategy must support primary capability")
	})

	t.Run("secondary missing CapSecondary capability", func(t *testing.T) {
		priConfig := compMockConfig{id: StrategyID(1), caps: CapPrimary}
		secConfig := compMockConfig{id: StrategyID(2), caps: CapPrimary} // Wrong capability

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for secondary missing CapSecondary")
		require.Contains(t, err.Error(), "secondary strategy must support secondary capability")
	})

	t.Run("unregistered primary strategy ID", func(t *testing.T) {
		priConfig := compMockConfig{id: StrategyID(200), caps: CapPrimary} // Unregistered ID
		secConfig := compMockConfig{id: StrategyID(2), caps: CapSecondary}

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for unregistered primary strategy")
		require.Contains(t, err.Error(), "failed to create primary strategy")
	})

	t.Run("unregistered secondary strategy ID", func(t *testing.T) {
		// Register a valid primary strategy
		priStrategy := &compMockStrategy{getRes: Results{"p": {Allowed: true}}}
		Register(StrategyID(1), func(_ backends.Backend) Strategy { return priStrategy })
		defer func() {
			registeredStrategies = make(map[StrategyID]StrategyFactory)
		}()

		priConfig := compMockConfig{id: StrategyID(1), caps: CapPrimary}
		secConfig := compMockConfig{id: StrategyID(201), caps: CapSecondary} // Unregistered ID

		_, err := NewComposite(storage, priConfig, secConfig)
		require.Error(t, err, "expected error for unregistered secondary strategy")
		require.Contains(t, err.Error(), "failed to create secondary strategy")
	})

	t.Run("successful creation", func(t *testing.T) {
		// Register valid strategies
		priStrategy := &compMockStrategy{getRes: Results{"p": {Allowed: true}}}
		secStrategy := &compMockStrategy{getRes: Results{"s": {Allowed: true}}}
		Register(StrategyID(1), func(_ backends.Backend) Strategy { return priStrategy })
		Register(StrategyID(2), func(_ backends.Backend) Strategy { return secStrategy })
		defer func() {
			registeredStrategies = make(map[StrategyID]StrategyFactory)
		}()

		priConfig := compMockConfig{id: StrategyID(1), caps: CapPrimary}
		secConfig := compMockConfig{id: StrategyID(2), caps: CapSecondary}

		composite, err := NewComposite(storage, priConfig, secConfig)
		require.NoError(t, err, "expected successful composite creation")
		require.NotNil(t, composite, "composite should not be nil")
	})
}
