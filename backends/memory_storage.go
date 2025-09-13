package backends

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStorage implements the Storage interface using in-memory storage
type MemoryStorage struct {
	mu     sync.RWMutex
	data   map[string]*State
	config BackendConfig
}

// NewMemoryStorage creates a new in-memory storage backend
func NewMemoryStorage(config BackendConfig) (*MemoryStorage, error) {
	storage := &MemoryStorage{
		data:   make(map[string]*State),
		config: config,
	}

	// Start cleanup routine if configured
	if config.CleanupInterval > 0 {
		go storage.cleanupRoutine(config.CleanupInterval)
	}

	return storage, nil
}

// Get retrieves state data for the given key
func (m *MemoryStorage) Get(ctx context.Context, key string) (*State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.data[key]
	if !exists {
		return nil, nil
	}

	// Return a copy to avoid concurrent modification issues
	return state.Clone(), nil
}

// Set stores state data for the given key with optional TTL
func (m *MemoryStorage) Set(ctx context.Context, key string, state *State, ttl time.Duration) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store a copy
	m.data[key] = state.Clone()

	// If TTL is specified, set expiration time
	if ttl > 0 {
		expiration := time.Now().Add(ttl)
		m.data[key].WithMetadata("expiration", expiration)
	}

	return nil
}

// CompareAndSet atomically updates state if it matches expected state
func (m *MemoryStorage) CompareAndSet(ctx context.Context, key string, expected *State, new *State, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, exists := m.data[key]

	// Check if current state matches expected
	if expected == nil {
		// Create operation - should only succeed if key doesn't exist
		if exists {
			return false, nil
		}
	} else {
		// Update operation - should only succeed if key exists and matches
		if !exists {
			return false, nil
		}

		// Simple comparison - in production you might want more sophisticated comparison
		if current.Counter != expected.Counter ||
			len(current.Timestamps) != len(expected.Timestamps) ||
			len(current.Values) != len(expected.Values) {
			return false, nil
		}
	}

	// Set new state
	m.data[key] = new.Clone()

	// If TTL is specified, set expiration time
	if ttl > 0 {
		expiration := time.Now().Add(ttl)
		m.data[key].WithMetadata("expiration", expiration)
	}

	return true, nil
}

// Delete removes state data for the given key
func (m *MemoryStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

// Close releases any resources held by the storage backend
func (m *MemoryStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear all data
	m.data = make(map[string]*State)
	return nil
}

// cleanupRoutine periodically removes expired keys
func (m *MemoryStorage) cleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupExpired()
	}
}

// cleanupExpired removes all expired keys
func (m *MemoryStorage) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, state := range m.data {
		if expiration, ok := state.Metadata["expiration"]; ok {
			if expTime, ok := expiration.(time.Time); ok && now.After(expTime) {
				delete(m.data, key)
			}
		}
	}
}

// GetStats returns statistics about the memory storage
func (m *MemoryStorage) GetStats() *MemoryStorageStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &MemoryStorageStats{
		TotalKeys:       int64(len(m.data)),
		CleanupInterval: m.config.CleanupInterval,
		MaxKeys:         m.config.MaxKeys,
	}

	// Calculate memory usage (rough estimate)
	for _, state := range m.data {
		stats.EstimatedMemory += int64(estimateStateSize(state))
	}

	return stats
}

// estimateStateSize provides a rough estimate of state memory usage
func estimateStateSize(state *State) int {
	size := 0
	size += 8                          // Counter (int64)
	size += len(state.Timestamps) * 24 // Each timestamp ~24 bytes
	size += len(state.Values) * 16     // Each float64 ~16 bytes
	size += len(state.Metadata) * 32   // Each metadata entry ~32 bytes (rough estimate)
	return size
}

// MemoryStorageStats holds statistics about the memory storage backend
type MemoryStorageStats struct {
	TotalKeys       int64
	EstimatedMemory int64
	CleanupInterval time.Duration
	MaxKeys         int
}

// GetAll returns all current state data (for testing/debugging)
func (m *MemoryStorage) GetAll() map[string]*State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*State)
	for key, state := range m.data {
		result[key] = state.Clone()
	}

	return result
}

// Clear removes all data from storage
func (m *MemoryStorage) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[string]*State)
	return nil
}
