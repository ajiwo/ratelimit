package postgres

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig     = errors.New("postgres backend requires postgres.Config")
	ErrInvalidConnString = errors.New("invalid postgres connection string")
	ErrInvalidPoolConfig = errors.New("invalid postgres pool configuration")

	// Connection errors
	ErrConnectionFailed   = errors.New("failed to connect to postgres")
	ErrPingFailed         = errors.New("failed to ping postgres server")
	ErrPoolCreationFailed = errors.New("failed to create connection pool")

	// Table/Schema errors
	ErrTableCreationFailed = errors.New("failed to create ratelimit table")
	ErrTableQueryFailed    = errors.New("failed to query ratelimit table")

	// Operation errors
	ErrSetFailed    = errors.New("failed to set key")
	ErrGetFailed    = errors.New("failed to get key")
	ErrDeleteFailed = errors.New("failed to delete key")
	ErrCloseFailed  = errors.New("failed to close postgres connection pool")

	// CheckAndSet specific errors
	ErrCheckAndSetFailed = errors.New("failed to perform check-and-set operation")
	ErrInvalidValueType  = errors.New("invalid value type for check-and-set")
)

// Configuration error functions
func NewInvalidConfigError(field string) error {
	return fmt.Errorf("postgres backend config: invalid %s", field)
}

func NewInvalidConnStringError(err error) error {
	return fmt.Errorf("invalid postgres connection string: %w", err)
}

func NewInvalidPoolConfigError(field string) error {
	return fmt.Errorf("invalid postgres pool configuration: %s", field)
}

// Connection error functions
func NewConnectionFailedError(err error) error {
	return fmt.Errorf("failed to connect to postgres: %w", err)
}

func NewPingFailedError(err error) error {
	return fmt.Errorf("postgres ping failed: %w", err)
}

func NewPoolCreationFailedError(err error) error {
	return fmt.Errorf("failed to create postgres connection pool: %w", err)
}

// Table/Schema error functions
func NewTableCreationFailedError(err error) error {
	return fmt.Errorf("failed to create ratelimit table: %w", err)
}

func NewTableQueryFailedError(query string, err error) error {
	return fmt.Errorf("failed to execute table query '%s': %w", query, err)
}

// Operation error functions
func NewGetFailedError(key string, err error) error {
	return fmt.Errorf("failed to get key '%s' from postgres: %w", key, err)
}

func NewSetFailedError(key string, err error) error {
	return fmt.Errorf("failed to set key '%s' in postgres: %w", key, err)
}

func NewDeleteFailedError(key string, err error) error {
	return fmt.Errorf("failed to delete key '%s' from postgres: %w", key, err)
}

func NewCloseFailedError(err error) error {
	return fmt.Errorf("failed to close postgres connection pool: %w", err)
}

// CheckAndSet error functions
func NewCheckAndSetFailedError(key string, err error) error {
	return fmt.Errorf("check-and-set operation failed for key '%s': %w", key, err)
}

func NewInvalidValueTypeError(key string, value any) error {
	return fmt.Errorf("invalid value type '%T' for check-and-set operation on key '%s'", value, key)
}
