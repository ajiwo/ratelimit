package memory

import (
	"context"
	"sync"
	"time"
)

const (
	// DefaultCleanupInterval is the default interval for cleaning up expired entries
	DefaultCleanupInterval = 10 * time.Minute
)

// mutexPool reduces allocations for mutex creation
var mutexPool = sync.Pool{
	New: func() any {
		return &sync.Mutex{}
	},
}

type Backend struct {
	locks         sync.Map     // map[string]*sync.Mutex
	values        sync.Map     // map[string]memoryValue
	cleanupTicker *time.Ticker // Ticker for periodic cleanup
	cleanupStop   chan bool    // Channel to stop cleanup goroutine
	cleanupWG     sync.WaitGroup
}

type memoryValue struct {
	value      string
	expiration time.Time
}

// New initializes a new in-memory storage instance with default (10 minutes) cleanup.
func New() *Backend {
	return NewWithCleanup(DefaultCleanupInterval)
}

// NewWithCleanup initializes a new in-memory storage instance with custom cleanup interval.
// Set interval to 0 to disable automatic cleanup.
func NewWithCleanup(interval time.Duration) *Backend {
	m := &Backend{
		cleanupStop: make(chan bool),
	}

	if interval > 0 {
		m.startCleanupRoutine(interval)
	}

	return m
}

// getLock returns a mutex for the given key using pool to reduce allocations
func (m *Backend) getLock(key string) *sync.Mutex {
	if existing, ok := m.locks.Load(key); ok {
		return existing.(*sync.Mutex)
	}

	// Use pooled mutex for new keys
	mutex := mutexPool.Get().(*sync.Mutex)
	actual, loaded := m.locks.LoadOrStore(key, mutex)
	if loaded {
		// Key already exists, return the pooled mutex to the pool
		mutexPool.Put(mutex)
	}
	return actual.(*sync.Mutex)
}

func (m *Backend) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	lock := m.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	valAny, exists := m.values.Load(key)
	if !exists {
		return "", nil
	}

	val := valAny.(memoryValue)
	if time.Now().After(val.expiration) {
		m.values.Delete(key) // Clean up expired key
		return "", nil
	}

	return val.value, nil
}

func (m *Backend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	lock := m.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	expirationTime := time.Now().Add(expiration)
	m.values.Store(key, memoryValue{
		value:      value,
		expiration: expirationTime,
	})

	return nil
}

func (m *Backend) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	lock := m.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	m.values.Delete(key)
	return nil
}

// startCleanupRoutine starts the cleanup goroutine with the given interval
func (m *Backend) startCleanupRoutine(interval time.Duration) {
	m.cleanupTicker = time.NewTicker(interval)
	m.cleanupWG.Go(m.runCleanupRoutine)
}

// runCleanupRoutine runs the cleanup goroutine
func (m *Backend) runCleanupRoutine() {
	for {
		select {
		case <-m.cleanupTicker.C:
			m.cleanup()
		case <-m.cleanupStop:
			return
		}
	}
}

// cleanup removes expired entries from storage
func (m *Backend) cleanup() {
	now := time.Now()
	var keysToDelete []string

	// First pass: find expired keys
	m.values.Range(func(key, valAny any) bool {
		val := valAny.(memoryValue)
		if now.After(val.expiration) {
			keysToDelete = append(keysToDelete, key.(string))
		}
		return true
	})

	// Second pass: delete expired keys with their individual locks
	for _, key := range keysToDelete {
		lock := m.getLock(key)
		lock.Lock()
		m.values.Delete(key)
		lock.Unlock()
	}
}

// Cleanup triggers an immediate cleanup of expired entries
// This method is exported for manual cleanup when needed
func (m *Backend) Cleanup() {
	m.cleanup()
}

func (m *Backend) Close() error {
	// Stop the cleanup ticker if it's running
	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
		if m.cleanupStop != nil {
			select {
			case <-m.cleanupStop:
				// Channel already closed
			default:
				close(m.cleanupStop)
			}
		}
	}

	m.cleanupWG.Wait()

	m.values = sync.Map{} // Clear the values map
	m.locks = sync.Map{}  // Clear the locks map
	return nil
}

// CheckAndSet atomically sets key to newValue only if current value matches oldValue.
// This operation provides compare-and-swap semantics for implementing optimistic locking.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - key: The storage key to operate on
//   - oldValue: Expected current value. Use empty string "" for "set if not exists" semantics
//   - newValue: New value to set if the current value matches oldValue
//   - expiration: Time-to-live for the key. Use 0 for no expiration
//
// Returns:
//   - bool: true if the operation succeeded (value was set), false otherwise
//   - error: Any storage-related error (not including failed comparison)
//
// Behavior:
//   - If oldValue is "", the operation succeeds only if the key doesn't exist
//   - If oldValue matches the current value, the key is updated to newValue
//   - Expired keys are treated as non-existent for comparison purposes
//   - All values are stored and compared as strings
func (m *Backend) CheckAndSet(ctx context.Context, key, oldValue, newValue string, expiration time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	lock := m.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	// Check if key exists and is not expired
	valAny, exists := m.values.Load(key)
	var val memoryValue
	if exists {
		val = valAny.(memoryValue)
		if time.Now().After(val.expiration) {
			// Key has expired, treat as non-existent
			exists = false
			m.values.Delete(key)
		}
	}

	if oldValue == "" {
		// Only set if key doesn't exist
		if exists {
			return false, nil
		}

		// Set new value
		expirationTime := time.Now().Add(expiration)
		m.values.Store(key, memoryValue{
			value:      newValue,
			expiration: expirationTime,
		})
		return true, nil
	}

	// Check if current value matches oldValue
	if !exists {
		return false, nil
	}

	if val.value != oldValue {
		return false, nil
	}

	// Value matches, update it
	expirationTime := time.Now().Add(expiration)
	m.values.Store(key, memoryValue{
		value:      newValue,
		expiration: expirationTime,
	})

	return true, nil
}
