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
	ErrContextCanceled  = errors.New("context canceled or timed out")
)

// Configuration validation error functions
func NewInvalidBurstError(burst int) error {
	return fmt.Errorf("token bucket burst must be positive, got %d", burst)
}

func NewInvalidRateError(rate float64) error {
	return fmt.Errorf("token bucket rate must be positive, got %f", rate)
}

// State operation error functions
func NewStateRetrievalError(err error) error {
	return fmt.Errorf("failed to get bucket state: %w", err)
}
