package gcra

import (
	"errors"
	"fmt"
)

var (
	// Configuration errors
	ErrInvalidConfig = errors.New("gcra strategy requires gcra.Config")
)

// Configuration validation error functions
func NewInvalidRateError(rate float64) error {
	return fmt.Errorf("gcra rate must be positive, got %f", rate)
}

func NewInvalidBurstError(burst int) error {
	return fmt.Errorf("gcra burst must be positive, got %d", burst)
}
