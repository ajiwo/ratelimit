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
