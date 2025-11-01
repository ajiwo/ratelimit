package fixedwindow

import (
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/strategies"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_Validate tests the validation logic for FixedWindow configuration.
func TestConfig_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "Valid config with single quota",
			config: Config{
				Key: "valid_single",
				Quotas: map[string]Quota{
					"per_second": {Limit: 10, Window: time.Second},
				},
			},
			expectError: false,
		},
		{
			name: "Valid config with multiple quotas",
			config: Config{
				Key: "valid_multiple",
				Quotas: map[string]Quota{
					"per_second": {Limit: 10, Window: time.Second},
					"per_minute": {Limit: 100, Window: time.Minute},
				},
			},
			expectError: false,
		},
		{
			name: "No quotas",
			config: Config{
				Key:    "no_quotas",
				Quotas: map[string]Quota{},
			},
			expectError: true,
		},
		{
			name: "Zero limit",
			config: Config{
				Key: "zero_limit",
				Quotas: map[string]Quota{
					"invalid": {Limit: 0, Window: time.Second},
				},
			},
			expectError: true,
		},
		{
			name: "Negative limit",
			config: Config{
				Key: "neg_limit",
				Quotas: map[string]Quota{
					"invalid": {Limit: -1, Window: time.Second},
				},
			},
			expectError: true,
		},
		{
			name: "Zero window",
			config: Config{
				Key: "zero_window",
				Quotas: map[string]Quota{
					"invalid": {Limit: 10, Window: 0},
				},
			},
			expectError: true,
		},
		{
			name: "Negative window",
			config: Config{
				Key: "neg_window",
				Quotas: map[string]Quota{
					"invalid": {Limit: 10, Window: -time.Second},
				},
			},
			expectError: true,
		},
		{
			name: "Duplicate rate ratio",
			config: Config{
				Key: "duplicate_rate",
				Quotas: map[string]Quota{
					"per_second": {Limit: 10, Window: time.Second},
					"equivalent": {Limit: 20, Window: 2 * time.Second},
				},
			},
			expectError: true,
		},
		{
			name: "More than 8 quotas",
			config: Config{
				Key: "too_many_quotas",
				Quotas: map[string]Quota{
					"q1": {Limit: 1, Window: time.Second},
					"q2": {Limit: 2, Window: time.Second},
					"q3": {Limit: 3, Window: time.Second},
					"q4": {Limit: 4, Window: time.Second},
					"q5": {Limit: 5, Window: time.Second},
					"q6": {Limit: 6, Window: time.Second},
					"q7": {Limit: 7, Window: time.Second},
					"q8": {Limit: 8, Window: time.Second},
					"q9": {Limit: 9, Window: time.Second}, // 9th quota - should fail
				},
			},
			expectError: true,
		},
		{
			name: "Empty key",
			config: Config{
				Key: "",
				Quotas: map[string]Quota{
					"per_second": {Limit: 10, Window: time.Second},
				},
			},
			expectError: false, // Empty key should be valid
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.expectError {
				assert.Error(t, err,
					"Expected validation error for invalid config: %+v", tc.config)
			} else {
				assert.NoError(t, err,
					"Unexpected validation error for valid config: %+v", tc.config)
			}
		})
	}
}

func TestConfig_Properties(t *testing.T) {
	config := Config{
		Key: "test_key",
		Quotas: map[string]Quota{
			"per_minute": {Limit: 100, Window: time.Minute},
		},
	}

	// Test ID()
	require.Equal(t, strategies.StrategyFixedWindow, config.ID(),
		"Config should return FixedWindow strategy ID")

	// Test Capabilities()
	caps := config.Capabilities()
	require.True(t, caps.Has(strategies.CapPrimary),
		"FixedWindow config should have primary capability")
	require.True(t, caps.Has(strategies.CapQuotas),
		"FixedWindow config should have quotas capability")

	// Test GetRole() and WithRole()
	require.Equal(t, strategies.RolePrimary, config.GetRole(),
		"FixedWindow config should have primary role by default")
	secondaryConfig := config.WithRole(strategies.RoleSecondary)
	require.Equal(t, strategies.RolePrimary, secondaryConfig.GetRole(),
		"WithRole should not change the role for FixedWindow (primary only)")

	// Test WithKey()
	updatedConfig := config.WithKey("new_key")
	require.Equal(t, "new_key", updatedConfig.(Config).Key,
		"WithKey should update the config key")
	require.Equal(t, config.Quotas, updatedConfig.(Config).Quotas,
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
	require.Equal(t, config.Quotas, config.GetQuotas(),
		"GetQuotas should return the correct quotas")
	require.Equal(t, 0, config.MaxRetries(),
		"MaxRetries should return the correct max retries")
}

func TestConfig_Builder(t *testing.T) {
	key := "builder_key"
	quotas := map[string]Quota{
		"q1": {Limit: 10, Window: time.Second},
		"q2": {Limit: 100, Window: time.Minute},
	}

	builder := NewConfig().
		SetKey(key).
		AddQuota("q1", 10, time.Second).
		AddQuota("q2", 100, time.Minute)

	config := builder.Build()

	require.Equal(t, key, config.Key, "Builder should set the key correctly")
	require.Equal(t, quotas, config.Quotas, "Builder should add quotas correctly")

	// Test building with no key
	noKeyConfig := NewConfig().AddQuota("q1", 1, time.Hour).Build()
	require.Empty(t, noKeyConfig.Key, "Key should be empty if not set")
	require.Len(t, noKeyConfig.Quotas, 1, "Should have one quota")
}
