package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type MemoryStorage struct {
	mu     sync.RWMutex
	values map[string]memoryValue
}

type memoryValue struct {
	value      any
	expiration time.Time
}

// New initializes a new in-memory storage instance.
func New() *MemoryStorage {
	return &MemoryStorage{
		values: make(map[string]memoryValue),
	}
}

func (m *MemoryStorage) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, exists := m.values[key]
	if !exists {
		return "", nil
	}

	if time.Now().After(val.expiration) {
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

func (m *MemoryStorage) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	expirationTime := time.Now().Add(expiration)
	m.values[key] = memoryValue{
		value:      value,
		expiration: expirationTime,
	}

	return nil
}

func (m *MemoryStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.values, key)
	return nil
}

func (m *MemoryStorage) cleanup() {
	now := time.Now()
	for key, val := range m.values {
		if now.After(val.expiration) {
			delete(m.values, key)
		}
	}
}

func (m *MemoryStorage) Close() error {
	// For in-memory storage, just clear the map and return nil
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values = make(map[string]memoryValue)
	return nil
}

// CheckAndSet atomically sets key to newValue only if current value matches oldValue
// Returns true if the set was successful, false if value didn't match or key expired
// oldValue=nil means "only set if key doesn't exist"
func (m *MemoryStorage) CheckAndSet(ctx context.Context, key string, oldValue, newValue any, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if key exists and is not expired
	val, exists := m.values[key]
	if exists && time.Now().After(val.expiration) {
		// Key has expired, treat as non-existent
		exists = false
	}

	if oldValue == nil {
		// Only set if key doesn't exist
		if exists {
			return false, nil
		}

		// Set new value
		expirationTime := time.Now().Add(expiration)
		m.values[key] = memoryValue{
			value:      newValue,
			expiration: expirationTime,
		}
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
	m.values[key] = memoryValue{
		value:      newValue,
		expiration: expirationTime,
	}

	return true, nil
}
