package fixedwindow

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig      = errors.New("fixed window strategy requires fixedwindow.Config")
	ErrNoQuotas           = errors.New("fixed window must have at least one quota")
	ErrDuplicateRateRatio = errors.New("duplicate rate ratio found")

	// State operation errors
	ErrStateParsing       = errors.New("failed to parse window state: invalid encoding")
	ErrStateRetrieval     = errors.New("failed to get window state")
	ErrStateSave          = errors.New("failed to save window state")
	ErrStateUpdate        = errors.New("failed to update window state")
	ErrConcurrentAccess   = errors.New("failed to update window state after max attempts due to concurrent access")
	ErrContextCancelled   = errors.New("context cancelled or timed out")
	ErrResetExpiredWindow = errors.New("failed to reset expired window")
)

// Configuration validation error functions
func NewInvalidQuotaLimitError(name string, limit int) error {
	return fmt.Errorf("fixed window quota '%s' limit must be positive, got %d", name, limit)
}

func NewInvalidQuotaWindowError(name string, window any) error {
	return fmt.Errorf("fixed window quota '%s' window must be positive, got %v", name, window)
}

func NewDuplicateRateRatioError(quota1, quota2 string, ratio float64) error {
	return fmt.Errorf("fixed window quotas '%s' and '%s' have duplicate rate ratios (both %.6f requests/second). Each quota must have a unique rate limit",
		quota1, quota2, ratio)
}

// State operation error functions
func NewStateRetrievalError(quotaName string, err error) error {
	return fmt.Errorf("failed to get window state for quota '%s': %w", quotaName, err)
}

func NewStateParsingError(quotaName string) error {
	return fmt.Errorf("failed to parse window state for quota '%s': invalid encoding", quotaName)
}

func NewStateSaveError(quotaName string, err error) error {
	return fmt.Errorf("failed to save window state for quota '%s': %w", quotaName, err)
}

func NewStateUpdateError(quotaName string, attempts int) error {
	return fmt.Errorf("failed to update window state for quota '%s' after %d attempts due to concurrent access", quotaName, attempts)
}

func NewContextCancelledError(err error) error {
	return fmt.Errorf("context cancelled or timed out: %w", err)
}

func NewResetExpiredWindowError(quotaName string, err error) error {
	return fmt.Errorf("failed to reset expired window for quota '%s': %w", quotaName, err)
}
