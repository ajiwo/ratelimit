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
func NewStateRetrievalError(quotaName string, err error) error {
	return fmt.Errorf("failed to get fixed window state for quota '%s': %w", quotaName, err)
}

func NewStateParsingError(quotaName string) error {
	return fmt.Errorf("failed to parse fixed window state for quota '%s': invalid encoding", quotaName)
}

func NewStateUpdateError(quotaName string, attempts int) error {
	return fmt.Errorf("failed to update fixed window state for quota '%s' after %d attempts due to concurrent access", quotaName, attempts)
}

func NewContextCanceledError(err error) error {
	return fmt.Errorf("context canceled or timed out: %w", err)
}
