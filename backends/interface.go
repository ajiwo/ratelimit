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

	// Delete removes a key from storage
	Delete(ctx context.Context, key string) error

	// Close releases resources used by the storage backend
	Close() error
}
