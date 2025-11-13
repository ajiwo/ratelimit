package composite

import (
	"context"
	"time"
)

// singleKeyAdapter implements an in-memory (fake) backend for single-key strategy execution
type singleKeyAdapter struct {
	value      string
	expiration time.Duration
}

// newSingleKeyAdapter creates and returns a new adapter with the specified initial value.
//
// not thread-safe, only use for single-threaded, single-use scenarios
func newSingleKeyAdapter(initialValue string) *singleKeyAdapter {
	return &singleKeyAdapter{
		value: initialValue,
	}
}

// Get returns the current value and nil error.
func (a *singleKeyAdapter) Get(_ context.Context, _ string) (string, error) {
	return a.value, nil
}

// Set stores the value and expiration and returns nil error.
func (a *singleKeyAdapter) Set(_ context.Context, _ string, value string, expiration time.Duration) error {
	a.value = value
	a.expiration = expiration
	return nil
}

// CheckAndSet compares the current value with oldValue and sets it to newValue if they match.
//
// It returns true and nil error if the operation succeeds, false and nil error otherwise.
func (a *singleKeyAdapter) CheckAndSet(_ context.Context, _ string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	if a.value != oldValue {
		return false, nil
	}
	a.value = newValue
	a.expiration = expiration
	return true, nil
}

// Delete clears the adapter state by calling reset and returns nil error.
func (a *singleKeyAdapter) Delete(_ context.Context, _ string) error {
	a.reset()
	return nil
}

func (a *singleKeyAdapter) reset() {
	a.value = ""
	a.expiration = 0
}

// Close returns nil error.
func (a *singleKeyAdapter) Close() error {
	return nil
}
