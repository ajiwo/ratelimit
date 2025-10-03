package backends

import (
	"context"
	"time"
)

// Backend defines the storage interface for atomic operations
type Backend interface {
	// Get retrieves a value from storage
	Get(ctx context.Context, key string) (string, error)

	// Set stores a value with expiration
	Set(ctx context.Context, key string, value any, expiration time.Duration) error

	// CheckAndSet atomically sets key to newValue only if current value matches oldValue
	// Returns true if the set was successful, false if value didn't match or key expired
	// oldValue=nil means "only set if key doesn't exist"
	CheckAndSet(ctx context.Context, key string, oldValue, newValue any, expiration time.Duration) (bool, error)

	// Delete removes a key from storage
	Delete(ctx context.Context, key string) error

	// Close releases resources used by the storage backend
	Close() error
}
