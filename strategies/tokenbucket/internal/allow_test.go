package internal

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockBackendOne is a mock implementation of the backends.Backend interface
type mockBackendOne struct {
	mock.Mock
}

func (m *mockBackendOne) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *mockBackendOne) CheckAndSet(ctx context.Context, key string, oldValue, newValue string, ttl time.Duration) (bool, error) {
	args := m.Called(ctx, key, oldValue, newValue, ttl)
	return args.Bool(0), args.Error(1)
}

func (m *mockBackendOne) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *mockBackendOne) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	args := m.Called(ctx, key, value, ttl)
	return args.Error(0)
}

func (m *mockBackendOne) Close() error {
	args := m.Called()
	return args.Error(0)
}

// mockConfigOne is a mock implementation of the Config interface
type mockConfigOne struct {
	mock.Mock
}

func (m *mockConfigOne) GetKey() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockConfigOne) GetBurstSize() int {
	args := m.Called()
	return args.Int(0)
}

func (m *mockConfigOne) GetRefillRate() float64 {
	args := m.Called()
	return args.Get(0).(float64)
}

func (m *mockConfigOne) MaxRetries() int {
	args := m.Called()
	return args.Int(0)
}

func TestAllow(t *testing.T) {
	ctx := context.Background()
	key := "test-key"
	burstSize := 10
	refillRate := 0.1
	maxRetries := 3

	t.Run("ReadOnly mode", func(t *testing.T) {
		t.Run("empty storage", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return("", nil)

			res, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
			assert.Equal(t, burstSize, res.Remaining)
			assert.False(t, res.stateUpdated)
		})

		t.Run("with existing state", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			lastRefill := time.Now().Add(-10 * time.Second)
			bucket := TokenBucket{
				Tokens:     5,
				LastRefill: lastRefill,
			}
			encodedState := encodeState(bucket)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			res, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)

			timeElapsed := time.Since(lastRefill).Seconds()
			tokensToAdd := timeElapsed * refillRate
			expectedTokens := int(math.Min(bucket.Tokens+tokensToAdd, float64(burstSize)))

			assert.Equal(t, expectedTokens > 0, res.Allowed)
			assert.Equal(t, expectedTokens, res.Remaining)
			assert.False(t, res.stateUpdated)
		})

		t.Run("state retrieval error", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return("", errors.New("storage error"))

			_, err := Allow(ctx, storage, config, ReadOnly)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get token bucket state")
		})

		t.Run("state parsing error", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return("invalid-state", nil)

			_, err := Allow(ctx, storage, config, ReadOnly)
			assert.Error(t, err)
			assert.Equal(t, ErrStateParsing, err)
		})
	})

	t.Run("TryUpdate mode", func(t *testing.T) {
		t.Run("empty storage", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return("", nil)

			newBucket := TokenBucket{
				Tokens:     float64(burstSize) - 1.0,
				LastRefill: time.Now(),
			}
			// We can't directly compare the newValue because LastRefill is time-sensitive
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil).Run(func(args mock.Arguments) {
				newValue := args.Get(3).(string)
				decoded, _ := decodeState(newValue)
				assert.InDelta(t, newBucket.Tokens, decoded.Tokens, 1e-9)
			})

			res, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
			assert.Equal(t, burstSize-1, res.Remaining)
			assert.True(t, res.stateUpdated)
		})

		t.Run("with existing state - allowed", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			lastRefill := time.Now().Add(-10 * time.Second)
			bucket := TokenBucket{
				Tokens:     5,
				LastRefill: lastRefill,
			}
			encodedState := encodeState(bucket)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			storage.On("CheckAndSet", ctx, key, encodedState, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil)

			res, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
			assert.True(t, res.stateUpdated)
		})

		t.Run("with existing state - not allowed", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			lastRefill := time.Now()
			bucket := TokenBucket{
				Tokens:     0.5,
				LastRefill: lastRefill,
			}
			encodedState := encodeState(bucket)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			config.On("MaxRetries").Return(maxRetries)
			storage.On("Get", ctx, key).Return(encodedState, nil)

			res, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.False(t, res.Allowed)
			assert.Equal(t, 0, res.Remaining)
			assert.False(t, res.stateUpdated)
		})

		t.Run("concurrent access retry", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			config.On("MaxRetries").Return(maxRetries)

			// First Get returns empty
			storage.On("Get", ctx, key).Return("", nil).Once()
			// First CheckAndSet fails when empty oldValue is used
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(false, nil).Once()

			// Second Get returns some state
			lastRefill := time.Now()
			bucket := TokenBucket{Tokens: 8, LastRefill: lastRefill}
			encodedState := encodeState(bucket)
			storage.On("Get", ctx, key).Return(encodedState, nil).Once()
			// Second CheckAndSet succeeds
			storage.On("CheckAndSet", ctx, key, encodedState, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil).Once()

			res, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.True(t, res.Allowed)
			assert.True(t, res.stateUpdated)
		})

		t.Run("concurrent access max retries exceeded", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			config.On("MaxRetries").Return(maxRetries)

			storage.On("Get", ctx, key).Return("", nil)
			storage.On("CheckAndSet", ctx, key, "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(false, nil)

			_, err := Allow(ctx, storage, config, TryUpdate)
			assert.Error(t, err)
			assert.Equal(t, ErrConcurrentAccess, err)
		})

		t.Run("context canceled", func(t *testing.T) {
			storage := new(mockBackendOne)
			config := new(mockConfigOne)
			canceledCtx, cancel := context.WithCancel(ctx)
			cancel()

			config.On("GetKey").Return(key)
			config.On("GetBurstSize").Return(burstSize)
			config.On("MaxRetries").Return(maxRetries)
			config.On("GetRefillRate").Return(refillRate)
			config.On("MaxRetries").Return(maxRetries)
			config.On("MaxRetries").Return(maxRetries)

			_, err := Allow(canceledCtx, storage, config, TryUpdate)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "context canceled or timed out")
		})
	})
}

func Test_calculateResetTime(t *testing.T) {
	now := time.Now()
	refillRate := 0.1

	t.Run("tokens are sufficient", func(t *testing.T) {
		bucket := TokenBucket{Tokens: 1.5}
		resetTime := calculateResetTime(now, bucket, refillRate)
		assert.Equal(t, now, resetTime)
	})

	t.Run("tokens are needed", func(t *testing.T) {
		bucket := TokenBucket{Tokens: 0.5}
		tokensNeeded := 1.0 - bucket.Tokens
		timeToRefill := tokensNeeded / refillRate
		expectedResetTime := now.Add(time.Duration(timeToRefill * float64(time.Second)))

		resetTime := calculateResetTime(now, bucket, refillRate)
		assert.WithinDuration(t, expectedResetTime, resetTime, 1*time.Millisecond)
	})

	t.Run("no tokens needed", func(t *testing.T) {
		bucket := TokenBucket{Tokens: 0}
		tokensNeeded := 1.0
		timeToRefill := tokensNeeded / refillRate
		expectedResetTime := now.Add(time.Duration(timeToRefill * float64(time.Second)))

		resetTime := calculateResetTime(now, bucket, refillRate)
		assert.WithinDuration(t, expectedResetTime, resetTime, 1*time.Millisecond)
	})
}
