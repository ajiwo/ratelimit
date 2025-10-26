package tokenbucket

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig = errors.New("token bucket strategy requires tokenbucket.Config")

	// State operation errors
	ErrStateParsing     = errors.New("failed to parse bucket state: invalid encoding")
	ErrStateRetrieval   = errors.New("failed to get bucket state")
	ErrStateSave        = errors.New("failed to save bucket state")
	ErrStateUpdate      = errors.New("failed to update bucket state")
	ErrConcurrentAccess = errors.New("failed to update bucket state after max attempts due to concurrent access")
	ErrContextCanceled = errors.New("context canceled or timed out")
)

// Configuration validation error functions
func NewInvalidBurstSizeError(burstSize int) error {
	return fmt.Errorf("token bucket burst size must be positive, got %d", burstSize)
}

func NewInvalidRefillRateError(refillRate float64) error {
	return fmt.Errorf("token bucket refill rate must be positive, got %f", refillRate)
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
