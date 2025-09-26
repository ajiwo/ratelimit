package backends

import "errors"

var (
	ErrBackendNotFound = errors.New("backend not found")
	ErrInvalidConfig   = errors.New("invalid backend configuration")
)
