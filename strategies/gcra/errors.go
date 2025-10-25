package gcra

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig = errors.New("gcra strategy requires gcra.Config")

	// State operation errors
	ErrStateParsing     = errors.New("failed to parse GCRA state: invalid encoding")
	ErrStateRetrieval   = errors.New("failed to get GCRA state")
	ErrStateSave        = errors.New("failed to save GCRA state")
	ErrStateUpdate      = errors.New("failed to update GCRA state")
	ErrConcurrentAccess = errors.New("failed to update GCRA state after max attempts due to concurrent access")
	ErrContextCancelled = errors.New("context cancelled or timed out")
)

// Configuration validation error functions
func NewInvalidRateError(rate float64) error {
	return fmt.Errorf("gcra rate must be positive, got %f", rate)
}

func NewInvalidBurstError(burst int) error {
	return fmt.Errorf("gcra burst must be positive, got %d", burst)
}

// State operation error functions
func NewStateRetrievalError(err error) error {
	return fmt.Errorf("failed to get GCRA state: %w", err)
}

func NewStateSaveError(err error) error {
	return fmt.Errorf("failed to save GCRA state: %w", err)
}

func NewContextCancelledError(err error) error {
	return fmt.Errorf("context cancelled or timed out: %w", err)
}
