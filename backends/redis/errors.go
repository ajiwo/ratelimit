package redis

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig    = errors.New("redis backend requires redis.Config")
	ErrConnectionFailed = errors.New("failed to connect to redis")
	ErrPingFailed       = errors.New("failed to ping redis server")

	// Operation errors
	ErrKeyNotFound  = errors.New("key not found")
	ErrSetFailed    = errors.New("failed to set key")
	ErrGetFailed    = errors.New("failed to get key")
	ErrDeleteFailed = errors.New("failed to delete key")
	ErrCloseFailed  = errors.New("failed to close redis connection")
	ErrEvalFailed   = errors.New("failed to evaluate lua script")

	// CheckAndSet specific errors
	ErrCheckAndSetFailed = errors.New("failed to perform check-and-set operation")
	ErrInvalidValueType  = errors.New("invalid value type for check-and-set")
	ErrScriptSHAInvalid  = errors.New("invalid script SHA hash for lua script")
)

// Configuration error functions
func NewInvalidConfigError(field string) error {
	return fmt.Errorf("redis backend config: invalid %s", field)
}

func NewConnectionFailedError(addr string, err error) error {
	return fmt.Errorf("failed to connect to redis at %s: %w", addr, err)
}

func NewPingFailedError(err error) error {
	return fmt.Errorf("redis ping failed: %w", err)
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
	return fmt.Errorf("failed to close redis connection: %w", err)
}

func NewEvalFailedError(script string, err error) error {
	return fmt.Errorf("failed to evaluate lua script: %w", err)
}

// CheckAndSet error functions
func NewCheckAndSetFailedError(key string, err error) error {
	return fmt.Errorf("check-and-set operation failed for key '%s': %w", key, err)
}

func NewInvalidValueTypeError(key string, value any) error {
	return fmt.Errorf("invalid value type '%T' for check-and-set operation on key '%s'", value, key)
}
