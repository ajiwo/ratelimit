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
				Key:   "valid",
				Burst: 10,
				Rate:  1.0,
			},
			expectError: false,
		},
		{
			name: "Zero capacity",
			config: Config{
				Key:   "zero_capacity",
				Burst: 0,
				Rate:  1.0,
			},
			expectError: true, // Burst must be positive
		},
		{
			name: "Negative capacity",
			config: Config{
				Key:   "neg_capacity",
				Burst: -5,
				Rate:  1.0,
			},
			expectError: true, // Burst must be positive
		},
		{
			name: "Zero leak rate",
			config: Config{
				Key:   "zero_leak_rate",
				Burst: 10,
				Rate:  0.0,
			},
			expectError: true, // Rate must be positive
		},
		{
			name: "Negative leak rate",
			config: Config{
				Key:   "neg_leak_rate",
				Burst: 10,
				Rate:  -1.0,
			},
			expectError: true, // Rate must be positive
		},
		{
			name: "Empty key",
			config: Config{
				Key:   "",
				Burst: 10,
				Rate:  1.0,
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
		Key:   "test_key",
		Burst: 10,
		Rate:  5.0,
	}

	// Test ID()
	require.Equal(t, strategies.StrategyLeakyBucket, config.ID(),
		"Config should return LeakyBucket strategy ID")

	// Test Capabilities()
	require.True(t, config.Capabilities().Has(strategies.CapPrimary),
		"LeakyBucket config should have primary capability")
	require.True(t, config.Capabilities().Has(strategies.CapSecondary),
		"LeakyBucket config should have secondary capability")

	// Test WithKey()
	updatedConfig := config.WithKey("new_key")
	require.Equal(t, "new_key", updatedConfig.(*Config).Key,
		"WithKey should update the config key")
	require.Equal(t, config.Burst, updatedConfig.(*Config).Burst,
		"WithKey should not change other config properties")
	require.Equal(t, config.Rate, updatedConfig.(*Config).Rate,
		"WithKey should not change other config properties")
	require.Equal(t, "test_key", config.GetKey(),
		"WithKey should not modify the original config")

	// Test WithMaxRetries()
	retriesConfig := config.WithMaxRetries(10)
	require.Equal(t, 10, retriesConfig.GetMaxRetries(),
		"WithMaxRetries should update the max retries")
	require.Equal(t, 10, retriesConfig.(*Config).MaxRetries,
		"WithMaxRetries should update the max retries")
	require.Equal(t, 11, config.GetMaxRetries(),
		"WithMaxRetries should not modify the calculated config")

	// Test getters
	require.Equal(t, "test_key", config.GetKey(),
		"GetKey should return the correct key")
	require.Equal(t, 10, config.GetBurst(),
		"GetBurst should return the correct burst")
	require.Equal(t, 5.0, config.GetRate(),
		"GetRate should return the correct rate")
	require.Equal(t, 11, config.GetMaxRetries(),
		"MaxRetries should return the calculated max retries when unset or set to 0")
}
