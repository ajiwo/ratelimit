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
	allowRes map[string]Result
	allowErr error
	getRes   map[string]Result
	getErr   error
	resetErr error
}

func (m *compMockStrategy) Allow(ctx context.Context, cfg StrategyConfig) (map[string]Result, error) {
	return m.allowRes, m.allowErr
}
func (m *compMockStrategy) Peek(ctx context.Context, cfg StrategyConfig) (map[string]Result, error) {
	return m.getRes, m.getErr
}
func (m *compMockStrategy) Reset(ctx context.Context, cfg StrategyConfig) error { return m.resetErr }

// mock config implementing StrategyConfig

type compMockConfig struct {
	id    StrategyID
	caps  CapabilityFlags
	role  StrategyRole
	key   string
	valid error
}

func (m compMockConfig) Validate() error                           { return m.valid }
func (m compMockConfig) ID() StrategyID                            { return m.id }
func (m compMockConfig) Capabilities() CapabilityFlags             { return m.caps }
func (m compMockConfig) GetRole() StrategyRole                     { return m.role }
func (m compMockConfig) WithRole(role StrategyRole) StrategyConfig { m.role = role; return m }
func (m compMockConfig) WithKey(key string) StrategyConfig         { m.key = key; return m }

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
	if p, ok := applied.Primary.(compMockConfig); !ok || p.key != "fully:qualified" {
		t.Fatalf("primary key not applied")
	}
	if s, ok := applied.Secondary.(compMockConfig); !ok || s.key != "fully:qualified" {
		t.Fatalf("secondary key not applied")
	}
}

func TestCompositeStrategyFlows(t *testing.T) {
	var storage backends.Backend = nil
	comp := NewComposite(storage)
	setter, ok := comp.(interface {
		SetPrimaryStrategy(Strategy)
		SetSecondaryStrategy(Strategy)
	})
	require.True(t, ok, "composite does not expose setters")

	pri := &compMockStrategy{getRes: map[string]Result{"p": {Allowed: true}}, allowRes: map[string]Result{"p": {Allowed: true}}}
	sec := &compMockStrategy{getRes: map[string]Result{"s": {Allowed: true}}, allowRes: map[string]Result{"s": {Allowed: true}}}

	setter.SetPrimaryStrategy(pri)
	setter.SetSecondaryStrategy(sec)

	cfg := CompositeConfig{BaseKey: "k", Primary: compMockConfig{caps: CapPrimary}, Secondary: compMockConfig{caps: CapSecondary}}

	// GetResult aggregates results with prefixes
	res, err := comp.Peek(context.Background(), cfg)
	require.NoError(t, err, "GetResult error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary_p in results")
	require.Contains(t, res, "secondary_s", "expected secondary_s in results")

	// Allow path where both allow
	_, err = comp.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)

	// Deny via primary
	primDeny := &compMockStrategy{getRes: map[string]Result{"p": {Allowed: false}}}
	setter.SetPrimaryStrategy(primDeny)
	res, err = comp.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "primary_p", "expected primary results present on denial")

	// Primary allows, secondary denies
	setter.SetPrimaryStrategy(&compMockStrategy{getRes: map[string]Result{"p": {Allowed: true}}})
	setter.SetSecondaryStrategy(&compMockStrategy{getRes: map[string]Result{"s": {Allowed: false}}})
	res, err = comp.Allow(context.Background(), cfg)
	require.NoError(t, err, "Allow error: %v", err)
	require.Contains(t, res, "secondary_s", "expected secondary results present on denial")

	// Error paths
	setter.SetPrimaryStrategy(&compMockStrategy{getErr: errors.New("bad primary")})
	_, err = comp.Allow(context.Background(), cfg)
	require.Error(t, err, "expected primary get error")

	setter.SetPrimaryStrategy(&compMockStrategy{getRes: map[string]Result{"p": {Allowed: true}}})
	setter.SetSecondaryStrategy(&compMockStrategy{getErr: errors.New("bad secondary")})
	_, err = comp.Allow(context.Background(), cfg)
	require.Error(t, err, "expected secondary get error")

	// Reset forwards to both strategies, error bubbles
	setter.SetPrimaryStrategy(&compMockStrategy{})
	setter.SetSecondaryStrategy(&compMockStrategy{resetErr: errors.New("reset fail")})
	err = comp.Reset(context.Background(), cfg)
	require.Error(t, err, "expected reset error")
}
