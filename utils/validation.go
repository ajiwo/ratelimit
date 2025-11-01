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

// ValidationOptions defines the validation rules for a string
type ValidationOptions struct {
	FieldName              string // Name of the field for error messages
	MaxLength              int    // Maximum allowed length
	MinLength              int    // Minimum allowed length (0 means no minimum)
	EmptyAllowed           bool   // Whether empty strings are allowed
	AdditionalAllowedChars string // Additional characters beyond the default set
}

// ValidateString validates a string against the given options
func ValidateString(value string, opts ValidationOptions) error {
	// Check empty string
	if !opts.EmptyAllowed && len(value) == 0 {
		return fmt.Errorf("%s cannot be empty", opts.FieldName)
	}

	if opts.EmptyAllowed && len(value) == 0 {
		return nil
	}

	// Check minimum length
	if opts.MinLength > 0 && len(value) < opts.MinLength {
		return fmt.Errorf("%s must be at least %d characters, got %d", opts.FieldName, opts.MinLength, len(value))
	}

	// Check maximum length
	if opts.MaxLength > 0 && len(value) > opts.MaxLength {
		return fmt.Errorf("%s cannot exceed %d bytes, got %d bytes", opts.FieldName, opts.MaxLength, len(value))
	}

	const hint = "Only alphanumeric ASCII, underscore (_), hyphen (-), colon (:), period (.), at (@), and plus (+) are allowed"

	// Create allowed characters set
	allowedChars := make(map[rune]bool)

	// Add default allowed characters
	for _, c := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-:.@+" {
		allowedChars[c] = true
	}

	// Add additional allowed characters if specified
	for _, c := range opts.AdditionalAllowedChars {
		allowedChars[c] = true
	}

	// Validate each character
	for i, r := range value {
		// Check if character is within ASCII range
		if r >= 128 {
			return fmt.Errorf("%s contains invalid character '%c' at position %d. %s", opts.FieldName, r, i, hint)
		}

		// Check if character is allowed
		if !allowedChars[r] {
			return fmt.Errorf("%s contains invalid character '%c' at position %d. %s", opts.FieldName, r, i, hint)
		}
	}

	return nil
}

// ValidateKey validates a rate limiting key with standard rules
func ValidateKey(key string) error {
	opts := ValidationOptions{
		FieldName:    "key",
		MaxLength:    64,
		MinLength:    1,
		EmptyAllowed: false,
	}
	return ValidateString(key, opts)
}

// ValidateQuotaName validates a quota name with standard rules
func ValidateQuotaName(name string) error {
	opts := ValidationOptions{
		FieldName:    "quota name",
		MaxLength:    64,
		MinLength:    1,
		EmptyAllowed: false,
	}
	return ValidateString(name, opts)
}
