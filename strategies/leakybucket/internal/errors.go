package internal

import (
	"errors"
	"fmt"
)

var (
	ErrStateParsing     = errors.New("failed to parse leaky bucket state: invalid encoding")
	ErrConcurrentAccess = errors.New("failed to update leaky bucket state after max attempts due to concurrent access")
)

// State operation error functions
func NewStateRetrievalError(err error) error {
	return fmt.Errorf("failed to get leaky bucket state: %w", err)
}

func NewStateSaveError(err error) error {
	return fmt.Errorf("failed to save leaky bucket state: %w", err)
}

func NewContextCanceledError(err error) error {
	return fmt.Errorf("context canceled or timed out: %w", err)
}
