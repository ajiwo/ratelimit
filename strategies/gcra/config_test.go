package gcra

import (
	"testing"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_Validate tests the validation logic for GCRA configuration.
// It verifies that valid configurations pass validation while invalid configurations fail.
func TestConfig_Validate(t *testing.T) {
	ctx := t.Context()
	storage := &mockBackend{store: make(map[string]string)}
	defer storage.Close()
	strategy := New(storage)

	testCases := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "Valid config",
			config: Config{
				Key:   "valid",
				Rate:  10.0,
				Burst: 5,
			},
			expectError: false,
		},
		{
			name: "Zero rate",
			config: Config{
				Key:   "zero_rate",
				Rate:  0.0,
				Burst: 5,
			},
			expectError: true, // Rate must be positive
		},
		{
			name: "Negative rate",
			config: Config{
				Key:   "neg_rate",
				Rate:  -5.0,
				Burst: 5,
			},
			expectError: true, // Rate must be positive
		},
		{
			name: "Zero burst",
			config: Config{
				Key:   "zero_burst",
				Rate:  10.0,
				Burst: 0,
			},
			expectError: true, // Burst must be positive
		},
		{
			name: "Negative burst",
			config: Config{
				Key:   "neg_burst",
				Rate:  10.0,
				Burst: -1,
			},
			expectError: true, // Burst must be positive
		},
		{
			name: "Empty key",
			config: Config{
				Key:   "",
				Rate:  10.0,
				Burst: 5,
			},
			expectError: false, // Empty key should be valid (handled by RateLimiter wrapper)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Validate config first
			err := tc.config.Validate()
			if tc.expectError {
				assert.Error(t, err, "Expected validation error for invalid config: %+v", tc.config)
			} else {
				assert.NoError(t, err, "Unexpected validation error for valid config: %+v", tc.config)

				// If config validation passes, test strategy call
				_, err := strategy.Allow(ctx, tc.config)
				assert.NoError(t, err, "Unexpected strategy error for valid config: %+v", tc.config)
			}
		})
	}
}

func TestConfig_Properties(t *testing.T) {
	config := Config{
		Key:   "pri",
		Burst: 2,
		Rate:  5.0,
	}

	// Verify that the config returns the correct strategy ID
	require.Equal(t, strategies.StrategyGCRA, config.ID(),
		"Config should return GCRA strategy ID")

	// GCRA only supports primary role, for now.
	require.Equal(t, strategies.RolePrimary, config.GetRole(),
		"GCRA config should have primary role by default")

	require.True(t, config.Capabilities().Has(strategies.CapPrimary),
		"GCRA config should have primary capability")

	// Verify that attempting to change to secondary role doesn't work (GCRA only supports primary)
	stillPrimary := config.WithRole(strategies.RoleSecondary)
	require.Equal(t, strategies.RolePrimary, stillPrimary.GetRole(),
		"GCRA config should maintain primary role regardless of requested role",
	)

	// Test WithKey method to verify it properly updates the key
	updatedConfig := config.WithKey("new_key")
	require.Equal(t, "new_key", updatedConfig.(Config).Key,
		"WithKey should update the config key")

	require.Equal(t, 2, updatedConfig.(Config).Burst,
		"WithKey should not change other config properties")

	require.Equal(t, 5.0, updatedConfig.(Config).Rate,
		"WithKey should not change other config properties")
}
