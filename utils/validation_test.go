package utils

import (
	"strings"
	"testing"
)

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		keyType     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid key",
			key:         "api_key_123",
			keyType:     "key",
			expectError: false,
		},
		{
			name:        "valid key with special characters",
			key:         "user:domain@host-123",
			keyType:     "key",
			expectError: false,
		},
		{
			name:        "empty key",
			key:         "",
			keyType:     "key",
			expectError: true,
			errorMsg:    "key cannot be empty",
		},
		{
			name:        "key too long",
			key:         "this_is_a_very_long_key_that_exceeds_the_maximum_allowed_length_of_sixty_four_characters_total",
			keyType:     "key",
			expectError: true,
			errorMsg:    "key cannot exceed 64 bytes",
		},
		{
			name:        "key with invalid characters",
			key:         "api key with spaces",
			keyType:     "key",
			expectError: true,
			errorMsg:    "key contains invalid character",
		},
		{
			name:        "key with non-ASCII",
			key:         "key_©2023",
			keyType:     "key",
			expectError: true,
			errorMsg:    "key contains invalid character",
		},
		{
			name:        "valid quota name",
			key:         "api_requests",
			keyType:     "quota name",
			expectError: false,
		},
		{
			name:        "valid quota name with special characters",
			key:         "api-requests:per-minute",
			keyType:     "quota name",
			expectError: false,
		},
		{
			name:        "valid quota name with plus",
			key:         "read+write_operations",
			keyType:     "quota name",
			expectError: false,
		},
		{
			name:        "empty quota name",
			key:         "",
			keyType:     "quota name",
			expectError: true,
			errorMsg:    "quota name cannot be empty",
		},
		{
			name:        "quota name too long",
			key:         "this_is_a_very_long_quota_name_that_exceeds_the_maximum_allowed_length_of_sixty_four_characters",
			keyType:     "quota name",
			expectError: true,
			errorMsg:    "quota name cannot exceed 64 bytes",
		},
		{
			name:        "quota name with spaces",
			key:         "invalid quota name",
			keyType:     "quota name",
			expectError: true,
			errorMsg:    "quota name contains invalid character",
		},
		{
			name:        "quota name with invalid characters",
			key:         "quota/name",
			keyType:     "quota name",
			expectError: true,
			errorMsg:    "quota name contains invalid character",
		},
		{
			name:        "quota name with non-ASCII",
			key:         "quota_©2023",
			keyType:     "quota name",
			expectError: true,
			errorMsg:    "quota name contains invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key, tt.keyType)
			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateKey() expected error but got none")
					return
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("ValidateKey() error message = %v, want to contain %v", err.Error(), tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("ValidateKey() unexpected error = %v", err)

			}
		})
	}
}

func TestValidateQuotaName(t *testing.T) {
	tests := []struct {
		name        string
		quotaName   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid quota name",
			quotaName:   "api_requests",
			expectError: false,
		},
		{
			name:        "valid quota name with special characters",
			quotaName:   "api-requests:per-minute",
			expectError: false,
		},
		{
			name:        "valid quota name with plus",
			quotaName:   "read+write_operations",
			expectError: false,
		},
		{
			name:        "empty quota name",
			quotaName:   "",
			expectError: true,
			errorMsg:    "quota name cannot be empty",
		},
		{
			name:        "quota name too long",
			quotaName:   "this_is_a_very_long_quota_name_that_exceeds_the_maximum_allowed_length_of_sixty_four_characters",
			expectError: true,
			errorMsg:    "quota name cannot exceed 64 bytes",
		},
		{
			name:        "quota name with spaces",
			quotaName:   "invalid quota name",
			expectError: true,
			errorMsg:    "quota name contains invalid character",
		},
		{
			name:        "quota name with invalid characters",
			quotaName:   "quota/name",
			expectError: true,
			errorMsg:    "quota name contains invalid character",
		},
		{
			name:        "quota name with non-ASCII",
			quotaName:   "quota_©2023",
			expectError: true,
			errorMsg:    "quota name contains invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuotaName(tt.quotaName)
			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateQuotaName() expected error but got none")
					return
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("ValidateQuotaName() error message = %v, want to contain %v", err.Error(), tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("ValidateQuotaName() unexpected error = %v", err)
			}
		})
	}
}

func TestAllowedChars(t *testing.T) {
	// Test that all expected characters are allowed
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-:.@+"

	for _, c := range validChars {
		err := ValidateKey(string(c), "test")
		if err != nil {
			t.Errorf("Character '%c' should be allowed but got error: %v", c, err)
		}
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("maximum length boundary", func(t *testing.T) {
		// Exactly 64 characters should be valid
		validLength := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789__"
		if len(validLength) != 64 {
			t.Fatalf("Test setup error: string length is %d, expected 64", len(validLength))
		}

		err := ValidateKey(validLength, "test")
		if err != nil {
			t.Errorf("Exactly 64 characters should be valid, got error: %v", err)
		}

		// 65 characters should be invalid
		invalidLength := validLength + "x"
		err = ValidateKey(invalidLength, "test")
		if err == nil {
			t.Error("65 characters should be invalid but got no error")
		}
	})
}
