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

func NewStateParsingError() error {
	return ErrStateParsing
}

func NewStateSaveError(err error) error {
	return fmt.Errorf("failed to save GCRA state: %w", err)
}

func NewStateUpdateError(attempts int) error {
	return fmt.Errorf("failed to update GCRA state after %d attempts due to concurrent access", attempts)
}

func NewContextCanceledError(err error) error {
	return fmt.Errorf("context canceled or timed out: %w", err)
}
