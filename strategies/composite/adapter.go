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

// newSingleKeyAdapter creates a new adapter with an initial value
//
// not thread-safe, only use for single-threaded, single-use scenarios
func newSingleKeyAdapter(initialValue string) *singleKeyAdapter {
	return &singleKeyAdapter{
		value: initialValue,
	}
}

func (a *singleKeyAdapter) Get(_ context.Context, _ string) (string, error) {
	return a.value, nil
}

func (a *singleKeyAdapter) Set(_ context.Context, _ string, value string, expiration time.Duration) error {
	a.value = value
	a.expiration = expiration
	return nil
}

func (a *singleKeyAdapter) CheckAndSet(_ context.Context, _ string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	if a.value != oldValue {
		return false, nil
	}
	a.value = newValue
	a.expiration = expiration
	return true, nil
}

func (a *singleKeyAdapter) Delete(_ context.Context, _ string) error {
	a.reset()
	return nil
}

func (a *singleKeyAdapter) reset() {
	a.value = ""
	a.expiration = 0
}

func (a *singleKeyAdapter) Close() error {
	return nil
}
