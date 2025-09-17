package backends

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

func NewMemoryStorage() *MemoryStorage {
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
