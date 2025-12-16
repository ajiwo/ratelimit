package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockBackend is a mock implementation of the backends.Backend interface
type mockBackend struct {
	mock.Mock
}

func (m *mockBackend) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *mockBackend) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, ttl time.Duration) (bool, error) {
	args := m.Called(ctx, key, oldValue, newValue, ttl)
	return args.Bool(0), args.Error(1)
}

func (m *mockBackend) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *mockBackend) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	args := m.Called(ctx, key, value, ttl)
	return args.Error(0)
}

func (m *mockBackend) Close() error {
	args := m.Called()
	return args.Error(0)
}

// mockConfig is a mock implementation of the Config interface
type mockConfig struct {
	mock.Mock
}

func (m *mockConfig) GetKey() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockConfig) GetBurst() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockConfig) GetRate() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

func (m *mockConfig) GetMaxRetries() int {
	args := m.Called()
	return args.Int(0)
}

func TestAllow(t *testing.T) {
	ctx := t.Context()
	key := "test-key"
	capacity := 10
	leakRate := 1.0 // 1 request per second
	maxRetries := 3

	t.Run("ReadOnly mode", func(t *testing.T) {
		t.Run("empty storage", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)
			storage.On("Get", ctx, key).Return("", nil)

			result, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.Equal(t, capacity, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("with existing state", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			lastLeak := time.Now().Add(-10 * time.Second)
			bucket := LeakyBucket{
				Requests: 5.0,
				LastLeak: lastLeak,
			}
			encodedState := encodeState(bucket)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			result, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)

			timeElapsed := time.Since(lastLeak).Seconds()
			requestsToLeak := timeElapsed * leakRate
			expectedRequests := max(0.0, bucket.Requests-requestsToLeak)
			expectedRemaining := max(capacity-int(expectedRequests), 0)

			assert.Equal(t, expectedRemaining > 0, result.Allowed)
			assert.Equal(t, expectedRemaining, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("state retrieval error", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)
			storage.On("Get", ctx, key).Return("", errors.New("storage error"))

			_, err := Allow(ctx, storage, config, ReadOnly)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get leaky bucket state")
		})

		t.Run("state parsing error", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)
			storage.On("Get", ctx, key).Return("invalid-state", nil)

			_, err := Allow(ctx, storage, config, ReadOnly)
			assert.Error(t, err)
			assert.Equal(t, ErrStateParsing, err)
		})
	})

	t.Run("TryUpdate mode", func(t *testing.T) {
		t.Run("empty storage - allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)
			storage.On("Get", ctx, key).Return("", nil)

			// Mock CheckAndSet with empty oldValue to succeed
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil)

			result, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.Equal(t, capacity-1, result.Remaining)
			assert.True(t, result.stateUpdated)
		})

		t.Run("with existing state - allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			lastLeak := time.Now().Add(-5 * time.Second)
			bucket := LeakyBucket{
				Requests: 3.0,
				LastLeak: lastLeak,
			}
			encodedState := encodeState(bucket)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			storage.On("CheckAndSet", ctx, key, encodedState, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil)

			result, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.True(t, result.stateUpdated)
		})

		t.Run("with existing state - not allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			bucket := LeakyBucket{
				Requests: float64(capacity),                     // At capacity
				LastLeak: time.Now().Add(-1 * time.Millisecond), // Very recent so minimal leaking
			}
			encodedState := encodeState(bucket)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			result, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.False(t, result.Allowed)
			assert.LessOrEqual(t, result.Remaining, 1) // Some leaking might occur
			assert.False(t, result.stateUpdated)
		})

		t.Run("concurrent access retry", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)

			// First Get returns empty
			storage.On("Get", ctx, key).Return("", nil).Once()
			// First CheckAndSet fails
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(false, nil).Once()

			// Second Get returns some state
			lastLeak := time.Now()
			bucket := LeakyBucket{Requests: 5.0, LastLeak: lastLeak}
			encodedState := encodeState(bucket)
			storage.On("Get", ctx, key).Return(encodedState, nil).Once()
			// Second CheckAndSet succeeds
			storage.On("CheckAndSet", ctx, key, encodedState, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil).Once()

			result, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.True(t, result.stateUpdated)
		})

		t.Run("concurrent access max retries exceeded", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)

			storage.On("Get", ctx, key).Return("", nil)
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(false, nil)

			_, err := Allow(ctx, storage, config, TryUpdate)
			assert.Error(t, err)
			assert.Equal(t, ErrConcurrentAccess, err)
		})

		t.Run("context canceled", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)
			canceledCtx, cancel := context.WithCancel(ctx)
			cancel()

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(capacity)
			config.On("GetMaxRetries").Return(maxRetries)
			config.On("GetRate").Return(leakRate)

			_, err := Allow(canceledCtx, storage, config, TryUpdate)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "context canceled")
		})
	})
}

func TestCalculateResetTime(t *testing.T) {
	now := time.Now()
	leakRate := 2.0 // 2 requests per second
	capacity := 10

	t.Run("bucket has capacity", func(t *testing.T) {
		bucket := LeakyBucket{Requests: 5.0} // Less than capacity
		resetTime := calculateResetTime(now, bucket, capacity, leakRate)
		assert.Equal(t, now, resetTime)
	})

	t.Run("bucket at capacity", func(t *testing.T) {
		bucket := LeakyBucket{Requests: 10.0} // At capacity
		requestsToLeak := bucket.Requests - float64(capacity) + 1
		timeToLeakSeconds := requestsToLeak / leakRate
		expectedResetTime := now.Add(time.Duration(timeToLeakSeconds * float64(time.Second)))

		resetTime := calculateResetTime(now, bucket, capacity, leakRate)
		assert.WithinDuration(t, expectedResetTime, resetTime, 1*time.Millisecond)
	})

	t.Run("bucket over capacity", func(t *testing.T) {
		bucket := LeakyBucket{Requests: 12.0} // Over capacity
		requestsToLeak := bucket.Requests - float64(capacity) + 1
		timeToLeakSeconds := requestsToLeak / leakRate
		expectedResetTime := now.Add(time.Duration(timeToLeakSeconds * float64(time.Second)))

		resetTime := calculateResetTime(now, bucket, capacity, leakRate)
		assert.WithinDuration(t, expectedResetTime, resetTime, 1*time.Millisecond)
	})
}
