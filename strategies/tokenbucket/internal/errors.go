package internal

import (
	"errors"
	"fmt"
)

var (
	ErrStateParsing     = errors.New("failed to parse token bucket state: invalid encoding")
	ErrConcurrentAccess = errors.New("failed to update token bucket state after max attempts due to concurrent access")
)

func NewStateRetrievalError(err error) error {
	return fmt.Errorf("failed to get token bucket state: %w", err)
}

func NewStateSaveError(err error) error {
	return fmt.Errorf("failed to save token bucket state: %w", err)
}

func NewContextCanceledError(err error) error {
	return fmt.Errorf("context canceled or timed out: %w", err)
}
