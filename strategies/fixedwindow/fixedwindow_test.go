package fixedwindow

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
	mu    sync.RWMutex
	store map[string]string
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		store: make(map[string]string),
	}
}

func (m *mockBackend) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if val, exists := m.store[key]; exists {
		return val, nil
	}
	return "", nil
}

func (m *mockBackend) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value.(string)
	return nil
}

func (m *mockBackend) CheckAndSet(ctx context.Context, key string, oldValue, newValue any, expiration time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, exists := m.store[key]
	if oldValue == nil && !exists {
		// Set only if key doesn't exist
		m.store[key] = newValue.(string)
		return true, nil
	}

	if exists && current == oldValue.(string) {
		m.store[key] = newValue.(string)
		return true, nil
	}

	return false, nil
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

func TestFixedWindow_Allow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config := NewConfig().
			SetKey("test-key").
			AddQuota("default", 5, time.Minute).
			Build()

		ctx := t.Context()

		// First 5 requests should be allowed
		for i := range 5 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// 6th request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "6th request should be denied")
	})
}

func TestFixedWindow_WindowReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config := NewConfig().
			SetKey("test-key").
			AddQuota("default", 2, time.Second).
			Build()

		ctx := t.Context()

		// Use up the limit
		for i := range 2 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// 3rd request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "3rd request should be denied")

		// Advance time past the window duration
		time.Sleep(1100 * time.Millisecond)
		synctest.Wait()

		// New request should be allowed in new window
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request in new window should be allowed")
	})
}

func TestFixedWindow_MultipleKeys(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config1 := NewConfig().
			SetKey("user1").
			AddQuota("default", 1, time.Minute).
			Build()

		config2 := NewConfig().
			SetKey("user2").
			AddQuota("default", 1, time.Minute).
			Build()

		ctx := t.Context()

		// First request for user1 should be allowed
		result, err := strategy.Allow(ctx, config1)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "First request for user1 should be allowed")

		// Second request for user1 should be denied
		result, err = strategy.Allow(ctx, config1)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Second request for user1 should be denied")

		// First request for user2 should be allowed (different key)
		result, err = strategy.Allow(ctx, config2)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "First request for user2 should be allowed")
	})
}

func TestFixedWindow_ZeroLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config := NewConfig().
			SetKey("test-key").
			AddQuota("default", 0, time.Minute).
			Build()

		ctx := t.Context()

		// Any request should be denied when limit is 0
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied when limit is 0")
	})
}

func TestFixedWindow_Peek(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config := NewConfig().
			SetKey("result-test-key").
			AddQuota("default", 5, time.Minute).
			Build()

		ctx := t.Context()

		// Test Peek with no existing data
		result, err := strategy.Peek(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should be allowed initially")
		assert.Equal(t, 5, result["default"].Remaining, "Remaining should be 5 initially")
		assert.WithinDuration(t, time.Now().Add(time.Minute), result["default"].Reset, time.Second, "Reset time should be approximately 1 minute from now")

		// Make some requests
		for range 3 {
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request should be allowed")
		}

		// Test Peek after requests
		result, err = strategy.Peek(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Result should still be allowed")
		assert.Equal(t, 2, result["default"].Remaining, "Remaining should be 2 after 3 requests")

		// Make remaining requests to hit the limit
		for range 2 {
			result, err = strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request should be allowed")
		}

		// Next request should be denied
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied")

		// Test Peek when at limit
		result, err = strategy.Peek(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Result should not be allowed when at limit")
		assert.Equal(t, 0, result["default"].Remaining, "Remaining should be 0 when at limit")
	})
}

func TestFixedWindow_Reset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config := NewConfig().
			SetKey("reset-test-key").
			AddQuota("default", 2, time.Minute).
			Build()

		ctx := t.Context()

		// Make requests to use up the limit
		for i := range 2 {
			result, err := strategy.Allow(ctx, config)
			require.NoError(t, err)
			assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
		}

		// Next request should be denied
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied (limit exceeded)")

		// Reset the counter
		err = strategy.Reset(ctx, config)
		require.NoError(t, err)

		// After reset, requests should be allowed again
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request after reset should be allowed")
	})
}

func TestFixedWindow_ConcurrentAccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config := NewConfig().
			SetKey("concurrent-key").
			AddQuota("default", 5, time.Minute).
			Build()

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
		result2, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result2["default"].Allowed, "11th request should be denied")
	})
}

func TestFixedWindow_PreciseTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		storage := newMockBackend()
		defer storage.Close()
		strategy := New(storage)

		config := NewConfig().
			SetKey("timing-key").
			AddQuota("default", 3, 5*time.Second).
			Build()

		ctx := t.Context()

		// Make requests at different times within the window
		result, err := strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "First request should be allowed")

		// Advance time by 2 seconds
		time.Sleep(2 * time.Second)
		synctest.Wait()

		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Second request should be allowed")

		// Advance time by 2 more seconds (4 seconds total)
		time.Sleep(2 * time.Second)
		synctest.Wait()

		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Third request should be allowed")

		// Advance time by 1 more second (5 seconds total, window should reset)
		time.Sleep(1 * time.Second)
		synctest.Wait()

		// Wait a bit more to ensure window reset
		time.Sleep(1 * time.Millisecond)
		synctest.Wait()

		// This request should be allowed because we're in a new window
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Fourth request should be allowed in new window")

		// Use up the rest of the limit in the new window
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Fifth request should be allowed")

		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Sixth request should be allowed")

		// This one should be denied
		result, err = strategy.Allow(ctx, config)
		require.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Seventh request should be denied")
	})
}

func TestFixedWindow_ValidationDuplicateRates(t *testing.T) {
	// Test 1: Valid unique ratios
	t.Run("valid unique ratios", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("burst", 10, time.Minute).    // 0.166667 req/sec
			AddQuota("sustained", 100, time.Hour). // 0.027778 req/sec
			AddQuota("daily", 1000, 24*time.Hour). // 0.011574 req/sec
			Build()
		err := config.Validate()
		assert.NoError(t, err, "Valid unique ratios should pass")
	})

	// Test 2: Invalid - same ratio, different windows
	t.Run("same ratio different windows", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("per-minute", 1, time.Minute). // 1/60 = 0.016667 req/sec
			AddQuota("per-hour", 60, time.Hour).    // 60/3600 = 0.016667 req/sec
			Build()
		err := config.Validate()
		assert.Error(t, err, "Same ratio with different windows should fail")
		assert.Contains(t, err.Error(), "duplicate rate ratios")
		assert.Contains(t, err.Error(), "per-minute")
		assert.Contains(t, err.Error(), "per-hour")
	})

	// Test 3: Invalid - different limits, same window
	t.Run("different limits same window", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("low", 10, time.Hour).   // 10/3600 = 0.002778 req/sec
			AddQuota("high", 100, time.Hour). // 100/3600 = 0.027778 req/sec
			Build()
		err := config.Validate()
		assert.NoError(t, err, "Different limits should pass (they have different rates)")
	})

	// Test 4: Valid - contradictory names but different rates
	t.Run("contradictory names different rates", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("slow", 1000, time.Hour). // 1000/3600 = 0.277778 req/sec
			AddQuota("fast", 50, time.Minute). // 50/60 = 0.833333 req/sec
			Build()
		err := config.Validate()
		assert.NoError(t, err, "Contradictory names with different rates should pass")
	})

	// Test 5: Valid - very close but different ratios
	t.Run("close but different ratios", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("quota-a", 100, time.Hour). // 100/3600 = 0.027778 req/sec
			AddQuota("quota-b", 101, time.Hour). // 101/3600 = 0.028056 req/sec
			Build()
		err := config.Validate()
		assert.NoError(t, err, "Very close but different ratios should pass")
	})

	// Test 6: Invalid - multiple duplicate ratios
	t.Run("multiple duplicate ratios", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("quota-a", 30, 30*time.Minute). // 30/1800 = 0.016667 req/sec
			AddQuota("quota-b", 60, time.Hour).      // 60/3600 = 0.016667 req/sec
			AddQuota("quota-c", 1440, 24*time.Hour). // 1440/86400 = 0.016667 req/sec
			Build()
		err := config.Validate()
		assert.Error(t, err, "Multiple duplicate ratios should fail")
		assert.Contains(t, err.Error(), "duplicate rate ratios")
	})

	// Test 7: Valid - single quota should always pass
	t.Run("single quota", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("single", 100, time.Hour).
			Build()
		err := config.Validate()
		assert.NoError(t, err, "Single quota should always pass")
	})

	// Test 8: Invalid - exact same limit and window
	t.Run("identical quotas", func(t *testing.T) {
		config := NewConfig().
			SetKey("test").
			AddQuota("quota1", 100, time.Hour).
			AddQuota("quota2", 100, time.Hour).
			Build()
		err := config.Validate()
		assert.Error(t, err, "Identical quotas should fail")
		assert.Contains(t, err.Error(), "duplicate rate ratios")
	})
}
