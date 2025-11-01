package utils

import (
	"testing"
)

func TestValidateString(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		opts        ValidationOptions
		expectError bool
		errorMsg    string
	}{
		{
			name:  "valid basic string",
			value: "test_key",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: false,
		},
		{
			name:  "valid string with special characters",
			value: "test-key:123@domain",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: false,
		},
		{
			name:  "valid string with plus sign",
			value: "test+key",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: false,
		},
		{
			name:  "empty string not allowed",
			value: "",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: true,
			errorMsg:    "test cannot be empty",
		},
		{
			name:  "empty string allowed",
			value: "",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    0,
				EmptyAllowed: true,
			},
			expectError: false,
		},
		{
			name:  "string too long",
			value: "this_is_a_very_long_string_that_exceeds_the_maximum_allowed_length_of_sixty_four_characters",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: true,
			errorMsg:    "cannot exceed 64 bytes",
		},
		{
			name:  "string too short",
			value: "ab",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    3,
				EmptyAllowed: false,
			},
			expectError: true,
			errorMsg:    "must be at least 3 characters",
		},
		{
			name:  "contains non-ASCII character",
			value: "test_key_©",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: true,
			errorMsg:    "contains invalid character",
		},
		{
			name:  "contains invalid special character",
			value: "test/key",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: true,
			errorMsg:    "contains invalid character",
		},
		{
			name:  "contains space",
			value: "test key",
			opts: ValidationOptions{
				FieldName:    "test",
				MaxLength:    64,
				MinLength:    1,
				EmptyAllowed: false,
			},
			expectError: true,
			errorMsg:    "contains invalid character",
		},
		{
			name:  "valid with additional allowed characters",
			value: "test/key",
			opts: ValidationOptions{
				FieldName:              "test",
				MaxLength:              64,
				MinLength:              1,
				EmptyAllowed:           false,
				AdditionalAllowedChars: "/",
			},
			expectError: false,
		},
		{
			name:  "valid with multiple additional allowed characters",
			value: "test/key$123",
			opts: ValidationOptions{
				FieldName:              "test",
				MaxLength:              64,
				MinLength:              1,
				EmptyAllowed:           false,
				AdditionalAllowedChars: "/$",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateString(tt.value, tt.opts)
			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateString() expected error but got none")
					return
				}
				if tt.errorMsg != "" && !containsSubstring(t, err.Error(), tt.errorMsg) {
					t.Errorf("ValidateString() error message = %v, want to contain %v", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateString() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid key",
			key:         "api_key_123",
			expectError: false,
		},
		{
			name:        "valid key with special characters",
			key:         "user:domain@host-123",
			expectError: false,
		},
		{
			name:        "empty key",
			key:         "",
			expectError: true,
			errorMsg:    "key cannot be empty",
		},
		{
			name:        "key too long",
			key:         "this_is_a_very_long_key_that_exceeds_the_maximum_allowed_length_of_sixty_four_characters_total",
			expectError: true,
			errorMsg:    "key cannot exceed 64 bytes",
		},
		{
			name:        "key with invalid characters",
			key:         "api key with spaces",
			expectError: true,
			errorMsg:    "key contains invalid character",
		},
		{
			name:        "key with non-ASCII",
			key:         "key_©2023",
			expectError: true,
			errorMsg:    "key contains invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if tt.expectError {
				if err == nil {
					t.Errorf("ValidateKey() expected error but got none")
					return
				}
				if tt.errorMsg != "" {
					if !containsSubstring(t, err.Error(), tt.errorMsg) {
						t.Errorf("ValidateKey() error message = %v, want to contain %v", err.Error(), tt.errorMsg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateKey() unexpected error = %v", err)
				}
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
				if tt.errorMsg != "" {
					if !containsSubstring(t, err.Error(), tt.errorMsg) {
						t.Errorf("ValidateQuotaName() error message = %v, want to contain %v", err.Error(), tt.errorMsg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("ValidateQuotaName() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestAllowedChars(t *testing.T) {
	// Test that all expected characters are allowed
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-:.@+"

	for _, c := range validChars {
		opts := ValidationOptions{
			FieldName:    "test",
			MaxLength:    64,
			MinLength:    1,
			EmptyAllowed: false,
		}

		err := ValidateString(string(c), opts)
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

		opts := ValidationOptions{
			FieldName:    "test",
			MaxLength:    64,
			MinLength:    1,
			EmptyAllowed: false,
		}

		err := ValidateString(validLength, opts)
		if err != nil {
			t.Errorf("Exactly 64 characters should be valid, got error: %v", err)
		}

		// 65 characters should be invalid
		invalidLength := validLength + "x"
		err = ValidateString(invalidLength, opts)
		if err == nil {
			t.Error("65 characters should be invalid but got no error")
		}
	})

	t.Run("minimum length boundary", func(t *testing.T) {
		opts := ValidationOptions{
			FieldName:    "test",
			MaxLength:    64,
			MinLength:    3,
			EmptyAllowed: false,
		}

		// Exactly 3 characters should be valid
		err := ValidateString("abc", opts)
		if err != nil {
			t.Errorf("Exactly 3 characters should be valid with MinLength=3, got error: %v", err)
		}

		// 2 characters should be invalid
		err = ValidateString("ab", opts)
		if err == nil {
			t.Error("2 characters should be invalid with MinLength=3 but got no error")
		}
	})
}

// Helper function to check if a string contains a substring
func containsSubstring(t *testing.T, s, substr string) bool {
	t.Helper()
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && findSubstring(t, s, substr)))
}

func findSubstring(t *testing.T, s, substr string) bool {
	t.Helper()
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
