package internal

import (
	"errors"
	"fmt"
)

var (
	ErrStateParsing     = errors.New("failed to parse GCRA state: invalid encoding")
	ErrConcurrentAccess = errors.New("failed to update GCRA state after max attempts due to concurrent access")
)

// State operation error functions
func NewStateRetrievalError(err error) error {
	return fmt.Errorf("failed to get GCRA state: %w", err)
}

func NewStateSaveError(err error) error {
	return fmt.Errorf("failed to save GCRA state: %w", err)
}

func NewContextCanceledError(err error) error {
	return fmt.Errorf("context canceled or timed out: %w", err)
}
