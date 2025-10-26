package tokenbucket

import (
	"testing"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_Validate tests the validation logic for TokenBucket configuration.
func TestConfig_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "Valid config",
			config: Config{
				Key:        "valid",
				BurstSize:  10,
				RefillRate: 1.0,
			},
			expectError: false,
		},
		{
			name: "Zero burst size",
			config: Config{
				Key:        "zero_burst",
				BurstSize:  0,
				RefillRate: 1.0,
			},
			expectError: true, // BurstSize must be positive
		},
		{
			name: "Negative burst size",
			config: Config{
				Key:        "neg_burst",
				BurstSize:  -5,
				RefillRate: 1.0,
			},
			expectError: true, // BurstSize must be positive
		},
		{
			name: "Zero refill rate",
			config: Config{
				Key:        "zero_refill_rate",
				BurstSize:  10,
				RefillRate: 0.0,
			},
			expectError: true, // RefillRate must be positive
		},
		{
			name: "Negative refill rate",
			config: Config{
				Key:        "neg_refill_rate",
				BurstSize:  10,
				RefillRate: -1.0,
			},
			expectError: true, // RefillRate must be positive
		},
		{
			name: "Empty key",
			config: Config{
				Key:        "",
				BurstSize:  10,
				RefillRate: 1.0,
			},
			expectError: false, // Empty key should be valid (handled by RateLimiter wrapper)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.expectError {
				assert.Error(t, err, "Expected validation error for invalid config: %+v", tc.config)
			} else {
				assert.NoError(t, err, "Unexpected validation error for valid config: %+v", tc.config)
			}
		})
	}
}

func TestConfig_Properties(t *testing.T) {
	config := Config{
		Key:        "test_key",
		BurstSize:  10,
		RefillRate: 5.0,
	}

	// Test ID()
	require.Equal(t, strategies.StrategyTokenBucket, config.ID(),
		"Config should return TokenBucket strategy ID")

	// Test Capabilities()
	require.True(t, config.Capabilities().Has(strategies.CapPrimary),
		"TokenBucket config should have primary capability")
	require.True(t, config.Capabilities().Has(strategies.CapSecondary),
		"TokenBucket config should have secondary capability")

	// Test GetRole() and WithRole()
	require.Equal(t, strategies.RolePrimary, config.GetRole(),
		"TokenBucket config should have primary role by default")
	secondaryConfig := config.WithRole(strategies.RoleSecondary)
	require.Equal(t, strategies.RoleSecondary, secondaryConfig.GetRole(),
		"WithRole should set the role to secondary")

	// check original is not modified
	require.Equal(t, strategies.RolePrimary, config.GetRole(),
		"WithRole should not modify the original config")

	// Test WithKey()
	updatedConfig := config.WithKey("new_key")
	require.Equal(t, "new_key", updatedConfig.(Config).Key,
		"WithKey should update the config key")
	require.Equal(t, config.BurstSize, updatedConfig.(Config).BurstSize,
		"WithKey should not change other config properties")
	require.Equal(t, config.RefillRate, updatedConfig.(Config).RefillRate,
		"WithKey should not change other config properties")
	require.Equal(t, "test_key", config.GetKey(),
		"WithKey should not modify the original config")

	// Test WithMaxRetries()
	retriesConfig := config.WithMaxRetries(10)
	require.Equal(t, 10, retriesConfig.MaxRetries(),
		"WithMaxRetries should update the max retries")
	require.Equal(t, 10, retriesConfig.(Config).maxRetries,
		"WithMaxRetries should update the max retries")
	require.Equal(t, 0, config.MaxRetries(),
		"WithMaxRetries should not modify the original config")

	// Test getters
	require.Equal(t, "test_key", config.GetKey(),
		"GetKey should return the correct key")
	require.Equal(t, 10, config.GetBurstSize(),
		"GetBurstSize should return the correct burst size")
	require.Equal(t, 5.0, config.GetRefillRate(),
		"GetRefillRate should return the correct refill rate")
	require.Equal(t, 0, config.MaxRetries(),
		"MaxRetries should return the correct max retries")
}
