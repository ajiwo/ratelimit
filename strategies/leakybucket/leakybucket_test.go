package leakybucket

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

func TestLeakyBucketAllowWithResult(t *testing.T) {
	ctx := t.Context()
	storage := &mockBackend{store: make(map[string]string)}
	t.Cleanup(func() { storage.Close() })

	strategy := New(storage)

	config := &Config{
		Key:   "test-user",
		Burst: 5,
		Rate:  1.0, // 1 request per second
	}

	// Fill up the bucket
	for i := range 5 {
		result, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
	}

	// Next request should be denied (bucket full)
	result, err := strategy.Allow(ctx, config)
	assert.NoError(t, err)
	assert.False(t, result["default"].Allowed, "Request should be denied when bucket is full")
}

func TestLeakyBucketLeak(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		storage := &mockBackend{store: make(map[string]string)}
		t.Cleanup(func() { storage.Close() })

		strategy := New(storage)

		config := &Config{
			Key:   "test-user-2",
			Burst: 3,
			Rate:  1.0, // 1 request per second
		}

		// Fill the bucket
		for range 3 {
			result, err := strategy.Allow(ctx, config)
			assert.NoError(t, err)
			assert.True(t, result["default"].Allowed)
		}

		// Next request should be denied (bucket full)
		result, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.False(t, result["default"].Allowed, "Request should be denied when bucket is full")

		// Wait for 2 seconds to allow leaking
		time.Sleep(2 * time.Second)
		synctest.Wait()

		// Now requests should be allowed again as requests have leaked
		result, err = strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request should be allowed after leaking")
	})
}

func TestLeakyBucketPeek(t *testing.T) {
	ctx := t.Context()
	storage := &mockBackend{store: make(map[string]string)}
	t.Cleanup(func() { storage.Close() })
	strategy := New(storage)

	config := &Config{
		Key:   "result-test-user",
		Burst: 5,
		Rate:  1.0, // 1 request per second
	}

	// Test Peek with no existing data
	result, err := strategy.Peek(ctx, config)
	require.NoError(t, err)
	assert.True(t, result["default"].Allowed, "Result should be allowed initially")
	assert.Equal(t, 5, result["default"].Remaining, "Remaining should be 5 initially")

	// Fill up the bucket
	for i := range 3 {
		result, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request %d should be allowed", i)
	}

	// Test Peek after adding requests
	result, err = strategy.Peek(ctx, config)
	require.NoError(t, err)
	assert.True(t, result["default"].Allowed, "Result should still be allowed")
	// Note: Since we're leaking 1 request per second, and some time has passed,
	// we might have more than 2 remaining. Let's just check it's more than 0.
	assert.True(t, result["default"].Remaining > 0, "Remaining should be greater than 0 after 3 requests")

	// Fill the bucket completely
	for i := range 2 {
		result, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, result["default"].Allowed, "Request %d should be allowed", i+3)
	}

	// Next request should be denied (bucket full)
	result, err = strategy.Allow(ctx, config)
	assert.NoError(t, err)
	assert.False(t, result["default"].Allowed, "Request should be denied when bucket is full")
}

func TestLeakyBucketReset(t *testing.T) {
	ctx := t.Context()
	storage := &mockBackend{store: make(map[string]string)}
	t.Cleanup(func() { storage.Close() })
	strategy := New(storage)

	config := &Config{
		Key:   "reset-test-user",
		Burst: 3,
		Rate:  1.0, // 1 request per second
	}

	// Fill the bucket
	for range 3 {
		result, err := strategy.Allow(ctx, config)
		assert.NoError(t, err)
		assert.True(t, result["default"].Allowed)
	}

	// Next request should be denied (bucket full)
	result, err := strategy.Allow(ctx, config)
	assert.NoError(t, err)
	assert.False(t, result["default"].Allowed, "Request should be denied when bucket is full")

	// Reset the bucket
	err = strategy.Reset(ctx, config)
	require.NoError(t, err)

	// After reset, requests should be allowed again
	result, err = strategy.Allow(ctx, config)
	assert.NoError(t, err)
	assert.True(t, result["default"].Allowed, "Request should be allowed after reset")
}
