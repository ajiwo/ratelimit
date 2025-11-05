package strategies

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStrategyID_String(t *testing.T) {
	cases := []struct {
		id ID
		s  string
	}{
		{StrategyTokenBucket, "token_bucket"},
		{StrategyFixedWindow, "fixed_window"},
		{StrategyLeakyBucket, "leaky_bucket"},
		{StrategyGCRA, "gcra"},
		{StrategyComposite, "composite"},
		{ID(255), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.id.String(); got != tc.s {
			require.Equal(t, tc.s, got, "id %v -> %q, want %q", tc.id, got, tc.s)
		}
	}
}

func TestCapabilityFlags_HasAndString(t *testing.T) {
	var f CapabilityFlags
	require.False(t, f.Has(CapPrimary))
	require.False(t, f.Has(CapSecondary))
	require.False(t, f.Has(CapQuotas))
	require.Equal(t, "None", f.String())

	f = CapPrimary
	require.True(t, f.Has(CapPrimary))
	require.False(t, f.Has(CapSecondary))
	require.False(t, f.Has(CapQuotas))

	f = CapSecondary | CapQuotas
	require.True(t, f.Has(CapSecondary))
	require.True(t, f.Has(CapQuotas))
	require.False(t, f.Has(CapPrimary))

	// String order should match implementation: Primary|Secondary|Quotas
	f = CapPrimary | CapSecondary | CapQuotas
	require.Equal(t, "Primary|Secondary|Quotas", f.String())
}
