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
	Set(ctx context.Context, key string, value string, expiration time.Duration) error

	// CheckAndSet atomically sets key to newValue only if current value matches oldValue.
	// This operation provides compare-and-swap (CAS) semantics for implementing optimistic locking.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - key: The storage key to operate on
	//   - oldValue: Expected current value. Use empty string "" for "set if not exists" semantics
	//   - newValue: New value to set if the current value matches oldValue
	//   - expiration: Time-to-live for the key. Use 0 for no expiration
	//
	// Returns:
	//   - bool: true if the CAS succeeded (value was set), false if the compare failed and no write occurred
	//   - error: Any storage-related error (not including a compare mismatch)
	//
	// Behavior:
	//   - If oldValue is "", the operation succeeds only if the key does not exist (or is expired)
	//   - If oldValue matches the current value, the key is updated to newValue
	//   - Expired keys are treated as non-existent for comparison purposes
	//   - All values are stored and compared as strings
	//
	// Caller contract:
	//   - A (false, nil) return indicates the compare condition did not match (e.g., another writer won the race
	//     or the key already exists when using "set if not exists"). This is not an error. Callers may safely
	//     reload state and retry with backoff according to their contention policy.
	//   - A non-nil error indicates a storage/backend failure and should not be retried blindly.
	CheckAndSet(ctx context.Context, key string, oldValue, newValue string, expiration time.Duration) (bool, error)

	// Delete removes a key from storage
	Delete(ctx context.Context, key string) error

	// Close releases resources used by the storage backend
	Close() error
}
