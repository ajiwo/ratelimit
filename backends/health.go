package backends

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrUnhealthy is a sentinel used to signal that the backend is unhealthy/unavailable.
var ErrUnhealthy = errors.New("backend unhealthy")

// HealthError wraps an underlying cause with operation context.
// Use for connectivity/auth/TLS/unavailability issues.
type HealthError struct {
	Op    string // logical operation context, e.g. "redis:ScriptLoad", "postgres:Ping", "get"
	Cause error  // underlying error returned by driver/client
}

// Error returns a formatted error message that includes the operation context and underlying cause.
// For a HealthError with Op="redis:Ping" and Cause="connection refused", this returns:
// "backend unhealthy: redis:Ping: connection refused"
// If Op is empty, it returns: "backend unhealthy: connection refused"
// If the receiver is nil, it returns the sentinel error message.
func (e *HealthError) Error() string {
	if e == nil {
		return ErrUnhealthy.Error()
	}
	if e.Op != "" {
		return fmt.Sprintf("%s: %s: %v", ErrUnhealthy, e.Op, e.Cause)
	}
	return fmt.Sprintf("%s: %v", ErrUnhealthy, e.Cause)
}

// Unwrap returns the underlying cause error, enabling error chaining with errors.Unwrap.
func (e *HealthError) Unwrap() error { return e.Cause }

// Is implements errors.Is to allow matching against ErrUnhealthy sentinel.
// This enables using errors.Is(err, ErrUnhealthy) to detect health errors.
func (e *HealthError) Is(target error) bool {
	return target == ErrUnhealthy
}

// NewHealthError wraps a cause as a health error with context.
// The op parameter should describe the logical operation being performed (e.g., "redis:Ping", "postgres:Get").
// If cause is nil, the sentinel ErrUnhealthy is returned.
// The returned error implements both errors.Is(ErrUnhealthy) and errors.As(*HealthError) for type checking.
func NewHealthError(op string, cause error) error {
	if cause == nil {
		return ErrUnhealthy
	}
	return &HealthError{Op: op, Cause: cause}
}

// IsHealthError reports whether err indicates backend is unhealthy.
// This function checks for both the ErrUnhealthy sentinel and any wrapped HealthError.
// It returns true for:
//   - ErrUnhealthy sentinel errors
//   - HealthError instances (direct or wrapped)
//   - Any error that wraps a HealthError via error chaining
//
// It returns false for regular operational errors that should not trigger failover.
func IsHealthError(err error) bool {
	// Check for sentinel error
	if errors.Is(err, ErrUnhealthy) {
		return true
	}
	// Check for HealthError type
	var he *HealthError
	return errors.As(err, &he)
}

// MaybeConnError checks if the error is a connectivity issue using the provided patterns.
//
// The op parameter should describe the operation being performed (e.g., "redis:Get", "postgres:Ping").
// patterns contains lowercase string patterns to match against error messages.
// If patterns is nil, no pattern matching is performed.
//
// Returns a HealthError if the error matches any pattern or is a context error,
// otherwise returns the original error.
//
// Examples:
//
//	err := MaybeConnError("myBackend:Connect", connErr, []string{"connection refused", "timeout"})
//	// Returns HealthError if error matches patterns
//
//	err := MaybeConnError("myBackend:Get", ioErr, nil)
//	// Always returns original error (no pattern matching)
func MaybeConnError(op string, err error, patterns []string) error {
	if err == nil {
		return nil
	}

	if patterns != nil {
		errStr := strings.ToLower(err.Error())
		for _, pattern := range patterns {
			if strings.Contains(errStr, pattern) {
				return NewHealthError(op, err)
			}
		}
	}

	// Check for context errors that indicate connectivity issues
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return NewHealthError(op, err)
	}

	return err
}
