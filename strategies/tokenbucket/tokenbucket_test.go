package tokenbucket

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBackend is a simple in-memory backend for testing
type mockBackend struct {
	store map[string]string
	mu    sync.RWMutex
}

func (m *mockBackend) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store[key], nil
}

func (m *mockBackend) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
	return nil
}

func (m *mockBackend) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.store[key]
	if oldValue == "" && exists {
		return false, nil
	}
	if oldValue != "" && (!exists || current != oldValue) {
		return false, nil
	}
	m.store[key] = newValue
	return true, nil
}

func (m *mockBackend) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	return nil
}

func (m *mockBackend) Close() error {
	return nil
}

func TestTokenBucket_Peek(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := &mockBackend{store: make(map[string]string)}
		t.Cleanup(func() { storage.Close() })
		strategy := New(storage)

		config := &Config{
			Key:   "result-test-key",
			Burst: 10,
			Rate:  10.0, // 10 tokens per second
		}

		// Test Peek with no existing data
		result, err := strategy.Peek(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should be allowed initially")
		assert.Equal(t, 10, result["default"].Remaining, "Remaining should be 10 initially")

		// Make some requests
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// Test Peek after requests
		result, err = strategy.Peek(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should still be allowed")
		assert.Equal(t, 5, result["default"].Remaining, "Remaining should be 5 after 5 requests")

		// Use all tokens
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i+5)
		}

		// Next request should be denied
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied when no tokens")
	})
}

func TestTokenBucket_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := &mockBackend{store: make(map[string]string)}
		t.Cleanup(func() { storage.Close() })
		strategy := New(storage)

		config := &Config{
			Key:   "reset-test-key",
			Burst: 4,
			Rate:  1.0, // 1 token per second
		}

		// Use all tokens
		for i := range 4 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// Next request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied (no tokens)")

		// Reset the bucket
		err = strategy.Reset(ctx, config)
		require.NoError(t, err)

		// After reset, requests should be allowed again
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request after reset should be allowed")
	})
}

func TestTokenBucket_Allow(t *testing.T) {
	// Test initial bucket should allow requests
	t.Run("initial bucket should allow requests", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := &mockBackend{store: make(map[string]string)}
			t.Cleanup(func() { storage.Close() })
			strategy := New(storage)

			config := &Config{
				Key:   "test_initial",
				Burst: 10,
				Rate:  10.0, // 10 tokens per second
			}

			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed)
		})
	})

	// Test should respect capacity limit
	t.Run("should respect capacity limit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := &mockBackend{store: make(map[string]string)}
			t.Cleanup(func() { storage.Close() })
			strategy := New(storage)

			config := &Config{
				Key:   "test_capacity",
				Burst: 3,
				Rate:  1.0, // 1 token per second
			}

			// First 3 requests should be allowed
			for i := range 3 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
			}

			// 4th request should be denied
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)
		})
	})

	// Test should handle multiple keys independently
	t.Run("should handle multiple keys independently", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := &mockBackend{store: make(map[string]string)}
			t.Cleanup(func() { storage.Close() })
			strategy := New(storage)

			config1 := &Config{
				Key:   "user1",
				Burst: 2,
				Rate:  2.0, // 2 tokens per second
			}
			config2 := &Config{
				Key:   "user2",
				Burst: 2,
				Rate:  2.0, // 2 tokens per second
			}

			// Use all tokens for key1
			for i := range 2 {
				result, err := strategy.Allow(ctx, config1)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Key1 request %d should be allowed", i)
			}

			// key1 should be denied
			result, err := strategy.Allow(ctx, config1)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)

			// key2 should still be allowed
			for i := range 2 {
				result, err := strategy.Allow(ctx, config2)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Key2 request %d should be allowed", i)
			}

			// key2 should now be denied
			result, err = strategy.Allow(ctx, config2)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)
		})
	})

	// Test fractional refill rate functionality
	t.Run("fractional refill rate functionality", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := t.Context()
			storage := &mockBackend{store: make(map[string]string)}
			t.Cleanup(func() { storage.Close() })
			strategy := New(storage)

			config := &Config{
				Key:   "test_fractional",
				Burst: 10,
				Rate:  2.0, // 2 tokens per second
			}

			// Use all tokens
			for i := range 10 {
				result, err := strategy.Allow(ctx, config)
				require.NoError(t, err)
				assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
			}

			// Next request should be denied
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.False(t, result["default"].Allowed)

			// Wait for some time to get fractional tokens
			time.Sleep(1200 * time.Millisecond)
			synctest.Wait()

			// At least one request should be allowed after waiting
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "At least one request should be allowed after fractional refill")
		})
	})
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := &mockBackend{store: make(map[string]string)}
		t.Cleanup(func() { storage.Close() })
		strategy := New(storage)

		config := &Config{
			Key:   "concurrent-key",
			Burst: 5,
			Rate:  5.0,
		}

		ctx := t.Context()

		// Start multiple goroutines that will try to make requests concurrently
		results := make(chan bool, 10)
		waitGroup := &sync.WaitGroup{}

		// Launch 10 goroutines that will all try to make a request
		for range 10 {
			waitGroup.Go(func() {
				result, err := strategy.Allow(ctx, config)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
					return
				}
				results <- result["default"].Allowed
			})
		}

		// Wait for all goroutines to complete
		waitGroup.Wait()
		close(results)

		// Collect results
		var allowedCount int
		for allowed := range results {
			if allowed {
				allowedCount++
			}
		}

		// Exactly 5 requests should be allowed (the limit)
		assert.Equal(t, 5, allowedCount, "Exactly 5 requests should be allowed")

		// Verify that we can't make any more requests
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "11th request should be denied")
	})
}
