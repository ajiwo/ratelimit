package backends

import "errors"

var (
	// ErrBackendNotFound is returned when attempting to create a backend with an unknown ID.
	ErrBackendNotFound = errors.New("backend not found")

	// ErrInvalidConfig is returned when the provided configuration is invalid.
	ErrInvalidConfig = errors.New("invalid backend configuration")
)
