package internal

import (
	"errors"
	"fmt"
)

var (
	ErrStateParsing     = errors.New("failed to parse fixed window state: invalid encoding")
	ErrConcurrentAccess = errors.New("failed to update fixed window state after max attempts due to concurrent access")
)

// State operation error functions
func NewStateRetrievalError(err error) error {
	return fmt.Errorf("failed to get fixed window state: %w", err)
}

func NewStateParsingError() error {
	return fmt.Errorf("failed to parse fixed window state: invalid encoding")
}

func NewStateUpdateError(attempts int) error {
	return fmt.Errorf("failed to update fixed window state after %d attempts due to concurrent access", attempts)
}

func NewContextCanceledError(err error) error {
	return fmt.Errorf("context canceled or timed out: %w", err)
}
