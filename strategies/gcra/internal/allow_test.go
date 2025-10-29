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

func (m *mockConfig) MaxRetries() int {
	args := m.Called()
	return args.Int(0)
}

func TestAllow(t *testing.T) {
	ctx := context.Background()
	key := "test-key"
	burst := 10
	rate := 10.0 // 10 requests per second
	maxRetries := 3

	t.Run("ReadOnly mode", func(t *testing.T) {
		t.Run("empty storage", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			storage.On("Get", ctx, key).Return("", nil)

			result, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.Equal(t, burst, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("with existing state - allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			// TAT is in the past, so we have capacity
			tat := time.Now().Add(-1 * time.Second)
			state := GCRA{TAT: tat}
			encodedState := encodeState(state)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			result, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.Greater(t, result.Remaining, 0)
			assert.False(t, result.stateUpdated)
		})

		t.Run("with existing state - not allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			// TAT is far in the future, exceeding the limit
			tat := time.Now().Add(5 * time.Second)
			state := GCRA{TAT: tat}
			encodedState := encodeState(state)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			result, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.False(t, result.Allowed)
			assert.Equal(t, 0, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("state retrieval error", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			storage.On("Get", ctx, key).Return("", errors.New("storage error"))

			_, err := Allow(ctx, storage, config, ReadOnly)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get GCRA state")
		})

		t.Run("state parsing error", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
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
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return("", nil)

			// Mock CheckAndSet with empty oldValue to succeed
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil)

			result, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.True(t, result.Allowed)
			assert.Greater(t, result.Remaining, 0)
			assert.True(t, result.stateUpdated)
		})

		t.Run("with existing state - allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			// TAT is in the past, so we have capacity
			tat := time.Now().Add(-500 * time.Millisecond)
			state := GCRA{TAT: tat}
			encodedState := encodeState(state)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			config.On("MaxRetries").Return(maxRetries)
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

			// TAT is far in the future, exceeding the limit
			tat := time.Now().Add(5 * time.Second)
			state := GCRA{TAT: tat}
			encodedState := encodeState(state)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			result, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.False(t, result.Allowed)
			assert.Equal(t, 0, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("concurrent access retry", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			config.On("MaxRetries").Return(maxRetries)

			// First Get returns empty
			storage.On("Get", ctx, key).Return("", nil).Once()
			// First CheckAndSet fails
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(false, nil).Once()

			// Second Get returns some state
			tat := time.Now().Add(-100 * time.Millisecond)
			state := GCRA{TAT: tat}
			encodedState := encodeState(state)
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
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			config.On("MaxRetries").Return(maxRetries)

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
			config.On("GetBurst").Return(burst)
			config.On("GetRate").Return(rate)
			config.On("MaxRetries").Return(maxRetries)

			_, err := Allow(canceledCtx, storage, config, TryUpdate)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "context canceled")
		})
	})
}

func TestCalculateRemaining(t *testing.T) {
	now := time.Now()
	emissionInterval := 100 * time.Millisecond // 10 requests per second
	limit := time.Second                       // 1 second limit
	burst := 10

	t.Run("full burst available - TAT in past", func(t *testing.T) {
		tat := now.Add(-1 * time.Second)
		remaining := calculateRemaining(now, tat, emissionInterval, limit, burst)
		assert.Equal(t, burst, remaining)
	})

	t.Run("partial burst available", func(t *testing.T) {
		// TAT is 300ms in the future, so we're 300ms behind
		tat := now.Add(300 * time.Millisecond)
		remaining := calculateRemaining(now, tat, emissionInterval, limit, burst)

		// We should have (1000ms - 300ms) / 100ms = 7 requests remaining
		expected := 7
		assert.Equal(t, expected, remaining)
	})

	t.Run("no requests available - at limit", func(t *testing.T) {
		// TAT is exactly at the limit
		tat := now.Add(limit)
		remaining := calculateRemaining(now, tat, emissionInterval, limit, burst)
		assert.Equal(t, 0, remaining)
	})

	t.Run("no requests available - over limit", func(t *testing.T) {
		// TAT exceeds the limit
		tat := now.Add(limit + 100*time.Millisecond)
		remaining := calculateRemaining(now, tat, emissionInterval, limit, burst)
		assert.Equal(t, 0, remaining)
	})

	t.Run("burst cap", func(t *testing.T) {
		// Even if calculation would exceed burst, cap it at burst
		tat := now.Add(-5 * time.Second) // Way in the past
		remaining := calculateRemaining(now, tat, emissionInterval, limit, burst)
		assert.Equal(t, burst, remaining)
	})
}
