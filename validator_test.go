package ratelimit

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateKey(t *testing.T) {
	testCases := []struct {
		name        string
		key         string
		keyType     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid alphanumeric key",
			key:         "user123",
			keyType:     "base key",
			expectError: false,
		},
		{
			name:        "Valid key with underscore",
			key:         "user_123",
			keyType:     "base key",
			expectError: false,
		},
		{
			name:        "Valid key with hyphen",
			key:         "user-123",
			keyType:     "base key",
			expectError: false,
		},
		{
			name:        "Valid key with colon",
			key:         "user:123",
			keyType:     "base key",
			expectError: false,
		},
		{
			name:        "Valid complex key",
			key:         "api:user:123_abc",
			keyType:     "dynamic key",
			expectError: false,
		},
		{
			name:        "Empty key",
			key:         "",
			keyType:     "base key",
			expectError: true,
			errorMsg:    "base key cannot be empty",
		},
		{
			name:        "Key too long - 65 bytes",
			key:         strings.Repeat("a", 65),
			keyType:     "dynamic key",
			expectError: true,
			errorMsg:    "dynamic key cannot exceed 64 bytes",
		},
		{
			name:        "Key with space",
			key:         "user 123",
			keyType:     "base key",
			expectError: true,
			errorMsg:    "contains invalid character ' '",
		},
		{
			name:        "Key with special character",
			key:         "user@123",
			keyType:     "dynamic key",
			expectError: true,
			errorMsg:    "contains invalid character '@'",
		},
		{
			name:        "Key with dot",
			key:         "user.123",
			keyType:     "base key",
			expectError: true,
			errorMsg:    "contains invalid character '.'",
		},
		{
			name:        "Key with slash",
			key:         "user/123",
			keyType:     "dynamic key",
			expectError: true,
			errorMsg:    "contains invalid character '/'",
		},
		{
			name:        "Key with non-ASCII character",
			key:         "üser123",
			keyType:     "base key",
			expectError: true,
			errorMsg:    "contains invalid character 'ü'",
		},
		{
			name:        "Key exactly 64 bytes",
			key:         strings.Repeat("a", 64),
			keyType:     "dynamic key",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateKey(tc.key, tc.keyType)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorMsg != "" {
					assert.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBaseKeyValidationInConfig(t *testing.T) {
	testCases := []struct {
		name        string
		baseKey     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid base key",
			baseKey:     "api:user:123",
			expectError: false,
		},
		{
			name:        "Invalid base key with space",
			baseKey:     "api user:123",
			expectError: true,
			errorMsg:    "contains invalid character ' '",
		},
		{
			name:        "Base key too long",
			baseKey:     strings.Repeat("x", 65),
			expectError: true,
			errorMsg:    "cannot exceed 64 bytes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := MultiTierConfig{
				BaseKey: tc.baseKey,
				Storage: nil, // This will cause another error, but we're testing key validation first
				Tiers: []TierConfig{
					{Interval: time.Minute, Limit: 10},
				},
			}

			err := validateBasicConfig(config)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				// If key is valid, we should get the storage backend error
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "storage backend cannot be nil")
			}
		})
	}
}

func TestDynamicKeyValidation(t *testing.T) {
	testCases := []struct {
		name        string
		dynamicKey  string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid dynamic key",
			dynamicKey:  "session:abc123",
			expectError: false,
		},
		{
			name:        "Invalid dynamic key with special char",
			dynamicKey:  "session@abc123",
			expectError: true,
			errorMsg:    "contains invalid character '@'",
		},
		{
			name:        "Empty dynamic key",
			dynamicKey:  "",
			expectError: true,
			errorMsg:    "dynamic key cannot be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			option := WithKey(tc.dynamicKey)
			opts := &accessOptions{}

			err := option(opts)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.dynamicKey, opts.key)
			}
		})
	}
}
