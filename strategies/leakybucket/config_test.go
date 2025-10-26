package leakybucket

import (
	"testing"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_Validate tests the validation logic for LeakyBucket configuration.
func TestConfig_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "Valid config",
			config: Config{
				Key:      "valid",
				Capacity: 10,
				LeakRate: 1.0,
			},
			expectError: false,
		},
		{
			name: "Zero capacity",
			config: Config{
				Key:      "zero_capacity",
				Capacity: 0,
				LeakRate: 1.0,
			},
			expectError: true, // Capacity must be positive
		},
		{
			name: "Negative capacity",
			config: Config{
				Key:      "neg_capacity",
				Capacity: -5,
				LeakRate: 1.0,
			},
			expectError: true, // Capacity must be positive
		},
		{
			name: "Zero leak rate",
			config: Config{
				Key:      "zero_leak_rate",
				Capacity: 10,
				LeakRate: 0.0,
			},
			expectError: true, // LeakRate must be positive
		},
		{
			name: "Negative leak rate",
			config: Config{
				Key:      "neg_leak_rate",
				Capacity: 10,
				LeakRate: -1.0,
			},
			expectError: true, // LeakRate must be positive
		},
		{
			name: "Empty key",
			config: Config{
				Key:      "",
				Capacity: 10,
				LeakRate: 1.0,
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
		Key:      "test_key",
		Capacity: 10,
		LeakRate: 5.0,
	}

	// Test ID()
	require.Equal(t, strategies.StrategyLeakyBucket, config.ID(),
		"Config should return LeakyBucket strategy ID")

	// Test Capabilities()
	require.True(t, config.Capabilities().Has(strategies.CapPrimary),
		"LeakyBucket config should have primary capability")
	require.True(t, config.Capabilities().Has(strategies.CapSecondary),
		"LeakyBucket config should have secondary capability")

	// Test GetRole() and WithRole()
	require.Equal(t, strategies.RolePrimary, config.GetRole(),
		"LeakyBucket config should have primary role by default")
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
	require.Equal(t, config.Capacity, updatedConfig.(Config).Capacity,
		"WithKey should not change other config properties")
	require.Equal(t, config.LeakRate, updatedConfig.(Config).LeakRate,
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
	require.Equal(t, 10, config.GetCapacity(),
		"GetCapacity should return the correct capacity")
	require.Equal(t, 5.0, config.GetLeakRate(),
		"GetLeakRate should return the correct leak rate")
	require.Equal(t, 0, config.MaxRetries(),
		"MaxRetries should return the correct max retries")
}
