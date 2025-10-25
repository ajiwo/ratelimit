package memory

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig = errors.New("memory backend requires memory.Config")
	ErrNilConfig     = errors.New("memory backend config cannot be nil")

	// Operation errors
	ErrSetFailed    = errors.New("failed to set key")
	ErrGetFailed    = errors.New("failed to get key")
	ErrDeleteFailed = errors.New("failed to delete key")
	ErrCloseFailed  = errors.New("failed to close memory backend")

	// CheckAndSet specific errors
	ErrCheckAndSetFailed = errors.New("failed to perform check-and-set operation")
	ErrInvalidValueType  = errors.New("invalid value type for check-and-set")
	ErrValueExpired      = errors.New("value has expired")

	// Internal errors
	ErrLockFailed      = errors.New("failed to acquire lock")
	ErrValueConversion = errors.New("failed to convert value to string")
)

// Configuration error functions
func NewInvalidConfigError(field string) error {
	return fmt.Errorf("memory backend config: invalid %s", field)
}

func NewNilConfigError() error {
	return errors.New("memory backend config cannot be nil")
}

// Operation error functions
func NewGetFailedError(key string, err error) error {
	return fmt.Errorf("failed to get key '%s': %w", key, err)
}

func NewSetFailedError(key string, err error) error {
	return fmt.Errorf("failed to set key '%s': %w", key, err)
}

func NewDeleteFailedError(key string, err error) error {
	return fmt.Errorf("failed to delete key '%s': %w", key, err)
}

func NewCloseFailedError(err error) error {
	return fmt.Errorf("failed to close memory backend: %w", err)
}

// CheckAndSet error functions
func NewCheckAndSetFailedError(key string, err error) error {
	return fmt.Errorf("check-and-set operation failed for key '%s': %w", key, err)
}

func NewInvalidValueTypeError(key string, value any) error {
	return fmt.Errorf("invalid value type '%T' for check-and-set operation on key '%s'", value, key)
}

func NewValueExpiredError(key string) error {
	return fmt.Errorf("key '%s' exists but value has expired", key)
}

// Internal error functions
func NewLockFailedError(key string, err error) error {
	return fmt.Errorf("failed to acquire lock for key '%s': %w", key, err)
}

func NewValueConversionError(key string, value any, err error) error {
	return fmt.Errorf("failed to convert value '%v' (type %T) to string for key '%s': %w", value, value, key, err)
}
