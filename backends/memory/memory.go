package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Backend struct {
	locks  sync.Map // map[string]*sync.Mutex
	values sync.Map // map[string]memoryValue
}

type memoryValue struct {
	value      any
	expiration time.Time
}

// New initializes a new in-memory storage instance.
func New() *Backend {
	return &Backend{
		// sync.Map doesn't need initialization with make()
	}
}

// getLock returns a mutex for the given key
func (m *Backend) getLock(key string) *sync.Mutex {
	actual, _ := m.locks.LoadOrStore(key, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func (m *Backend) Get(ctx context.Context, key string) (string, error) {
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

	switch v := val.value.(type) {
	case string:
		return v, nil
	case int:
		return fmt.Sprintf("%d", v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func (m *Backend) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
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
	lock := m.getLock(key)
	lock.Lock()
	defer lock.Unlock()

	m.values.Delete(key)
	return nil
}

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

func (m *Backend) Close() error {
	// For in-memory storage, just clear the map and return nil
	// Note: We don't need to acquire individual locks here since we're replacing the entire map
	m.values = sync.Map{} // Clear the values map
	m.locks = sync.Map{}  // Clear the locks map
	return nil
}

// CheckAndSet atomically sets key to newValue only if current value matches oldValue
// Returns true if the set was successful, false if value didn't match or key expired
// oldValue=nil means "only set if key doesn't exist"
func (m *Backend) CheckAndSet(ctx context.Context, key string, oldValue, newValue any, expiration time.Duration) (bool, error) {
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

	if oldValue == nil {
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

	// Convert both values to strings for comparison
	currentStr := fmt.Sprintf("%v", val.value)
	oldStr := fmt.Sprintf("%v", oldValue)

	if currentStr != oldStr {
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
