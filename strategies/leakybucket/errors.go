package leakybucket

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig = errors.New("leaky bucket strategy requires leakybucket.Config")

	// State operation errors
	ErrStateParsing     = errors.New("failed to parse bucket state: invalid encoding")
	ErrStateRetrieval   = errors.New("failed to get bucket state")
	ErrStateSave        = errors.New("failed to save bucket state")
	ErrStateUpdate      = errors.New("failed to update bucket state")
	ErrConcurrentAccess = errors.New("failed to update bucket state after max attempts due to concurrent access")
	ErrContextCanceled = errors.New("context canceled or timed out")
)

// Configuration validation error functions
func NewInvalidCapacityError(capacity int) error {
	return fmt.Errorf("leaky bucket capacity must be positive, got %d", capacity)
}

func NewInvalidLeakRateError(leakRate float64) error {
	return fmt.Errorf("leaky bucket leak rate must be positive, got %f", leakRate)
}

// State operation error functions
func NewStateRetrievalError(err error) error {
	return fmt.Errorf("failed to get bucket state: %w", err)
}

func NewStateSaveError(err error) error {
	return fmt.Errorf("failed to save bucket state: %w", err)
}

func NewContextCanceledError(err error) error {
	return fmt.Errorf("context canceled or timed out: %w", err)
}
