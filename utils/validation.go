package utils

import "fmt"

// allowedCharsArray is a precomputed boolean array for O(1) character validation
var allowedCharsArray [128]bool

func init() {
	// Initialize all characters as not allowed
	for i := range 128 {
		allowedCharsArray[i] = false
	}

	// Set allowed characters to true
	for _, c := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-:.@+" {
		allowedCharsArray[c] = true
	}
}

// ValidateKey validates that a key meets the requirements:
// - Maximum 64 bytes length
// - Contains only alphanumeric ASCII characters, underscore (_), hyphen (-), colon (:), period (.), at (@), and plus (+)
func ValidateKey(key, keyType string) error {
	if len(key) == 0 {
		return fmt.Errorf("%s cannot be empty", keyType)
	}

	if len(key) > 64 {
		return fmt.Errorf("%s cannot exceed 64 bytes, got %d bytes", keyType, len(key))
	}

	const hint = "Only alphanumeric ASCII, underscore (_), hyphen (-), colon (:), period (.), at (@), and plus (+) are allowed"

	for i, r := range key {
		// Check if character is within ASCII range
		if r >= 128 {
			return fmt.Errorf("%s contains invalid character '%c' at position %d. %s", keyType, r, i, hint)
		}

		// Check if character is allowed
		if !allowedCharsArray[r] {
			return fmt.Errorf("%s contains invalid character '%c' at position %d. %s", keyType, r, i, hint)
		}
	}

	return nil
}

// ValidateQuotaName validates a quota name with the same rules as keys
func ValidateQuotaName(name string) error {
	return ValidateKey(name, "quota name")
}
