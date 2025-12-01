package strategies

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResults(t *testing.T) {
	now := time.Now()
	reset := now.Add(1 * time.Hour)

	tests := []struct {
		name    string
		results Results
	}{
		{
			name:    "empty results",
			results: Results{},
		},
		{
			name: "single quota",
			results: Results{
				"default": {
					Allowed:   true,
					Remaining: 5,
					Reset:     reset,
				},
			},
		},
		{
			name: "multiple quotas",
			results: Results{
				"default": {
					Allowed:   true,
					Remaining: 10,
					Reset:     reset,
				},
				"hourly": {
					Allowed:   false,
					Remaining: 0,
					Reset:     reset.Add(1 * time.Hour),
				},
				"daily": {
					Allowed:   true,
					Remaining: 100,
					Reset:     reset.Add(24 * time.Hour),
				},
			},
		},
		{
			name: "dual strategy results",
			results: Results{
				"primary_default": {
					Allowed:   true,
					Remaining: 50,
					Reset:     reset,
				},
				"secondary_default": {
					Allowed:   true,
					Remaining: 8,
					Reset:     reset,
				},
				"primary_burst": {
					Allowed:   false,
					Remaining: 0,
					Reset:     reset,
				},
			},
		},
		{
			name: "mixed dual and regular quotas",
			results: Results{
				"default":           Result{Allowed: true, Remaining: 5, Reset: reset},
				"primary_default":   Result{Allowed: true, Remaining: 100, Reset: reset.Add(1 * time.Hour)},
				"secondary_default": Result{Allowed: false, Remaining: 0, Reset: reset},
				"primary_hourly":    Result{Allowed: true, Remaining: 99, Reset: reset.Add(1 * time.Hour)},
				"hourly":            Result{Allowed: false, Remaining: 0, Reset: reset.Add(1 * time.Hour)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.results

			// Test Len
			require.Equal(t, len(tt.results), r.Len())

			// Test Default
			defaultResult := r.Default()
			wantDefault := tt.results["default"]
			require.EqualValues(t, wantDefault, defaultResult, "Default() = %v, want %v", defaultResult, wantDefault)

			// Test PrimaryDefault
			primaryDefault := r.PrimaryDefault()
			wantPrimaryDefault := tt.results["primary_default"]
			require.EqualValues(t, wantPrimaryDefault, primaryDefault, "PrimaryDefault() = %v, want %v", primaryDefault, wantPrimaryDefault)

			// Test SecondaryDefault
			secondaryDefault := r.SecondaryDefault()
			wantSecondaryDefault := tt.results["secondary_default"]
			require.EqualValues(t, wantSecondaryDefault, secondaryDefault, "SecondaryDefault() = %v, want %v", secondaryDefault, wantSecondaryDefault)

			// Test Quota for existing quotas
			for quotaName := range tt.results {
				result := r.Quota(quotaName)
				want := tt.results[quotaName]
				require.EqualValues(t, want, result, "Quota(%q) = %v, want %v", quotaName, result, want)
			}

			// Test Quota for non-existing quota
			nonexistentResult := r.Quota("nonexistent")
			require.EqualValues(t, Result{}, nonexistentResult, "Quota(nonexistent) = %v, want empty Result", nonexistentResult)

			// Test Primary and Secondary helpers
			for quotaName := range tt.results {
				// Test Primary
				primaryResult := r.Primary(quotaName)
				wantPrimary := tt.results["primary_"+quotaName]
				require.EqualValues(t, wantPrimary, primaryResult, "Primary(%q) = %v, want %v", quotaName, primaryResult, wantPrimary)

				// Test Secondary
				secondaryResult := r.Secondary(quotaName)
				wantSecondary := tt.results["secondary_"+quotaName]
				require.EqualValues(t, wantSecondary, secondaryResult, "Secondary(%q) = %v, want %v", quotaName, secondaryResult, wantSecondary)
			}

			// Test HasQuota
			for quotaName := range tt.results {
				require.True(t, r.HasQuota(quotaName), "HasQuota(%q) should be true", quotaName)
			}
			require.False(t, r.HasQuota("nonexistent"), "HasQuota(nonexistent) should be false")

			// Test AllAllowed
			wantAllAllowed := true
			for _, result := range tt.results {
				if !result.Allowed {
					wantAllAllowed = false
					break
				}
			}
			require.Equal(t, wantAllAllowed, r.AllAllowed())

			// Test AnyAllowed
			wantAnyAllowed := false
			for _, result := range tt.results {
				if result.Allowed {
					wantAnyAllowed = true
					break
				}
			}
			require.Equal(t, wantAnyAllowed, r.AnyAllowed())

			// Test First - since map iteration order is not guaranteed,
			// we just check that it returns a valid result from the map
			if len(tt.results) > 0 {
				first := r.First()
				found := false
				for _, want := range tt.results {
					if want.Allowed == first.Allowed && want.Remaining == first.Remaining && want.Reset.Equal(first.Reset) {
						found = true
						break
					}
				}
				require.True(t, found, "First() returned result that doesn't exist in map: %v", first)
			} else {
				first := r.First()
				require.EqualValues(t, Result{}, first, "First() on empty map should return empty Result, got %v", first)
			}
		})
	}
}

func TestResultsEdgeCases(t *testing.T) {
	t.Run("empty results map", func(t *testing.T) {
		r := Results{}

		// All helper methods should return zero/empty values
		require.Equal(t, Result{}, r.Default())
		require.Equal(t, Result{}, r.PrimaryDefault())
		require.Equal(t, Result{}, r.SecondaryDefault())
		require.Equal(t, Result{}, r.Quota("any"))
		require.Equal(t, Result{}, r.Primary("any"))
		require.Equal(t, Result{}, r.Secondary("any"))
		require.False(t, r.HasQuota("any"))
		// AllAllowed returns true for empty maps (no restrictions)
		require.True(t, r.AllAllowed())
		require.False(t, r.AnyAllowed())
		require.Equal(t, 0, r.Len())
		require.Equal(t, Result{}, r.First())
	})

	t.Run("nil results map", func(t *testing.T) {
		var r Results

		// All methods should work with nil map
		require.Equal(t, Result{}, r.Default())
		require.Equal(t, 0, r.Len())
		// AllAllowed returns true for nil maps (no restrictions)
		require.True(t, r.AllAllowed())
		require.False(t, r.AnyAllowed())
	})

	t.Run("all denied", func(t *testing.T) {
		r := Results{
			"default": {Allowed: false},
			"hourly":  {Allowed: false},
		}

		require.False(t, r.AllAllowed())
		require.False(t, r.AnyAllowed())
	})

	t.Run("mixed allowed and denied", func(t *testing.T) {
		r := Results{
			"default": {Allowed: true},
			"hourly":  {Allowed: false},
			"daily":   {Allowed: true},
		}

		require.False(t, r.AllAllowed())
		require.True(t, r.AnyAllowed())
	})
}

func TestResultsDualStrategyHelpers(t *testing.T) {
	t.Run("complete dual strategy", func(t *testing.T) {
		r := Results{
			"primary_default":   {Allowed: true, Remaining: 100},
			"secondary_default": {Allowed: true, Remaining: 10},
			"primary_burst":     {Allowed: false, Remaining: 0},
			"secondary_burst":   {Allowed: true, Remaining: 5},
		}

		// Test Primary and Secondary with various names
		require.Equal(t, 100, r.Primary("default").Remaining)
		require.Equal(t, 10, r.Secondary("default").Remaining)
		require.Equal(t, 0, r.Primary("burst").Remaining)
		require.Equal(t, 5, r.Secondary("burst").Remaining)

		// Non-existent prefixed quotas
		require.Equal(t, Result{}, r.Primary("nonexistent"))
		require.Equal(t, Result{}, r.Secondary("nonexistent"))
	})

	t.Run("mixed prefix and non-prefix quotas", func(t *testing.T) {
		r := Results{
			"default":           {Allowed: true, Remaining: 5},
			"primary_default":   {Allowed: true, Remaining: 50},
			"secondary_default": {Allowed: true, Remaining: 8},
			"hourly":            {Allowed: false, Remaining: 0},
		}

		// Default() should return the non-prefixed default
		require.Equal(t, 5, r.Default().Remaining)

		// Primary/Secondary helpers should return prefixed versions
		require.Equal(t, 50, r.PrimaryDefault().Remaining)
		require.Equal(t, 8, r.SecondaryDefault().Remaining)

		// Direct Quota() should return exact matches
		require.Equal(t, 5, r.Quota("default").Remaining)
		require.Equal(t, 50, r.Quota("primary_default").Remaining)

		// Primary/Secondary helpers should add prefixes
		require.Equal(t, Result{}, r.Primary("hourly"))
		require.Equal(t, Result{}, r.Secondary("hourly"))
	})
}
