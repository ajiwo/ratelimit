package backends

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"
)

// copyMetadata creates a shallow copy of metadata map
func copyMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return make(map[string]any)
	}

	copy := make(map[string]any)
	maps.Copy(copy, metadata)
	return copy
}

// MemoryBackend implements the Backend interface using in-memory storage
type MemoryBackend struct {
	data            map[string]*BackendData
	mutex           sync.RWMutex
	cleanupInterval time.Duration
	maxKeys         int
	stopCleanup     chan struct{}
	cleanupDone     chan struct{}
}

// NewMemoryBackend creates a new memory backend
func NewMemoryBackend(config BackendConfig) (*MemoryBackend, error) {
	cleanupInterval := config.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 5 * time.Minute // Default cleanup interval
	}

	maxKeys := config.MaxKeys
	if maxKeys == 0 {
		maxKeys = 10000 // Default max keys
	}

	backend := &MemoryBackend{
		data:            make(map[string]*BackendData),
		cleanupInterval: cleanupInterval,
		maxKeys:         maxKeys,
		stopCleanup:     make(chan struct{}),
		cleanupDone:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go backend.cleanupLoop()

	return backend, nil
}

// Get retrieves rate limit data for the given key
func (m *MemoryBackend) Get(ctx context.Context, key string) (*BackendData, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	data, exists := m.data[key]
	if !exists {
		return nil, nil // Return nil for non-existent keys
	}

	// Return a copy to avoid race conditions
	return &BackendData{
		Count:       data.Count,
		WindowStart: data.WindowStart,
		LastRequest: data.LastRequest,
		Tokens:      data.Tokens,
		LastRefill:  data.LastRefill,
		Metadata:    copyMetadata(data.Metadata),
	}, nil
}

// Set stores rate limit data for the given key with optional TTL
func (m *MemoryBackend) Set(ctx context.Context, key string, data *BackendData, ttl time.Duration) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if we're at max capacity and this is a new key
	if _, exists := m.data[key]; !exists && len(m.data) >= m.maxKeys {
		return fmt.Errorf("memory backend at maximum capacity (%d keys)", m.maxKeys)
	}

	// Store a copy to avoid race conditions
	m.data[key] = &BackendData{
		Count:       data.Count,
		WindowStart: data.WindowStart,
		LastRequest: data.LastRequest,
		Tokens:      data.Tokens,
		LastRefill:  data.LastRefill,
		Metadata:    copyMetadata(data.Metadata),
	}

	// TTL is handled by the cleanup goroutine checking LastRequest
	// We don't need to set explicit expiration times

	return nil
}

// Increment atomically checks if incrementing would exceed the limit
func (m *MemoryBackend) Increment(ctx context.Context, key string, window time.Duration, limit int64) (int64, bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	data, exists := m.data[key]

	if !exists {
		// Check capacity for new key
		if len(m.data) >= m.maxKeys {
			return 0, false, fmt.Errorf("memory backend at maximum capacity (%d keys)", m.maxKeys)
		}

		// First request - always allowed if limit > 0
		if limit > 0 {
			m.data[key] = &BackendData{
				Count:       1,
				WindowStart: now,
				LastRequest: now,
				Tokens:      0,
				LastRefill:  now,
				Metadata:    make(map[string]any),
			}
			return 1, true, nil
		}
		return 0, false, nil
	}

	// Check if we need to reset the window
	if now.Sub(data.WindowStart) >= window {
		// Reset the window - first request in new window
		if limit > 0 {
			data.Count = 1
			data.WindowStart = now
			data.LastRequest = now
			return 1, true, nil
		}
		return 0, false, nil
	}

	// Check if incrementing would exceed limit
	if data.Count >= limit {
		// Would exceed limit - don't increment
		return data.Count, false, nil
	}

	// Increment within the current window
	data.Count++
	data.LastRequest = now
	return data.Count, true, nil
}

// ConsumeToken atomically checks if a token is available and consumes it
func (m *MemoryBackend) ConsumeToken(ctx context.Context, key string, bucketSize int64, refillRate time.Duration, refillAmount int64, window time.Duration) (float64, bool, int64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	data, exists := m.data[key]

	if !exists {
		// Check capacity for new key
		if len(m.data) >= m.maxKeys {
			return 0, false, 0, fmt.Errorf("memory backend at maximum capacity (%d keys)", m.maxKeys)
		}

		// First request - start with full bucket minus one token
		if bucketSize > 0 {
			m.data[key] = &BackendData{
				Count:       1,
				WindowStart: now.Add(-window), // Rolling window
				LastRequest: now,
				Tokens:      float64(bucketSize - 1), // Consume one token
				LastRefill:  now,
				Metadata:    make(map[string]any),
			}
			return float64(bucketSize - 1), true, 1, nil
		}
		return 0, false, 0, nil
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(data.LastRefill)

	// Calculate how many complete refill periods have passed
	if refillRate > 0 {
		refillPeriods := elapsed / refillRate
		if refillPeriods > 0 {
			tokensToAdd := float64(refillPeriods) * float64(refillAmount)
			data.Tokens = min(data.Tokens+tokensToAdd, float64(bucketSize))
			// Advance LastRefill by the duration of the periods we've just accounted for.
			// This preserves any fractional time for the next calculation.
			data.LastRefill = data.LastRefill.Add(refillPeriods * refillRate)
		}

		// Check if we can consume a token
		if data.Tokens >= 1 {
			// Consume one token
			data.Tokens--
			data.Count++
			data.LastRequest = now
			data.WindowStart = now.Add(-window) // Rolling window

			return data.Tokens, true, data.Count, nil
		}

		// Not enough tokens - update LastRequest but don't consume
		data.LastRequest = now
		return data.Tokens, false, data.Count, nil
	}

	// If refillRate is 0, no refill happens
	currentTokens := int64(data.Tokens)
	if currentTokens >= 1 {
		// Consume one token
		currentTokens -= 1
		data.Count++
		data.Tokens = float64(currentTokens)
		data.LastRequest = now
		data.WindowStart = now.Add(-window) // Rolling window

		return float64(currentTokens), true, data.Count, nil
	}

	// Not enough tokens
	data.LastRequest = now
	return float64(currentTokens), false, data.Count, nil
}

// Delete removes rate limit data for the given key
func (m *MemoryBackend) Delete(ctx context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.data, key)
	return nil
}

// Close releases any resources held by the backend
func (m *MemoryBackend) Close() error {
	// Signal cleanup goroutine to stop
	close(m.stopCleanup)

	// Wait for cleanup goroutine to finish
	<-m.cleanupDone

	// Clear data
	m.mutex.Lock()
	m.data = nil
	m.mutex.Unlock()

	return nil
}

// GetStats returns statistics about the memory backend
func (m *MemoryBackend) GetStats() MemoryBackendStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return MemoryBackendStats{
		KeyCount:        len(m.data),
		MaxKeys:         m.maxKeys,
		CleanupInterval: m.cleanupInterval,
	}
}

// MemoryBackendStats holds statistics about the memory backend
type MemoryBackendStats struct {
	KeyCount        int
	MaxKeys         int
	CleanupInterval time.Duration
}

// cleanupLoop runs periodically to clean up expired entries
func (m *MemoryBackend) cleanupLoop() {
	defer close(m.cleanupDone)

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopCleanup:
			return
		}
	}
}

// cleanup removes expired entries from the backend
func (m *MemoryBackend) cleanup() {
	// 1: Collect candidate keys under read lock
	m.mutex.RLock()
	now := time.Now()
	candidates := make([]string, 0)
	for key, data := range m.data {
		// A key is considered expired if it hasn't been accessed in two cleanup intervals,
		// providing a grace period before removal.
		if now.Sub(data.LastRequest) > m.cleanupInterval*2 {
			candidates = append(candidates, key)
		}
	}
	m.mutex.RUnlock()

	if len(candidates) == 0 {
		return
	}

	// 2: Re-check and delete under write lock to avoid check-then-act race
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, key := range candidates {
		if data, ok := m.data[key]; ok {
			// Re-validate with current time to ensure key is still expired
			if time.Since(data.LastRequest) > m.cleanupInterval*2 {
				delete(m.data, key)
			}
		}
	}
}
