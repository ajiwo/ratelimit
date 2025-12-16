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

func (m *mockConfig) GetQuotas() []Quota {
	args := m.Called()
	return args.Get(0).([]Quota)
}

func (m *mockConfig) GetMaxRetries() int {
	args := m.Called()
	return args.Int(0)
}

func TestAllow(t *testing.T) {
	ctx := t.Context()
	key := "test-key"
	quota := Quota{
		Limit:  10,
		Window: time.Minute,
	}
	quota.Name = "default"
	quotas := []Quota{quota}
	maxRetries := 3

	t.Run("ReadOnly mode", func(t *testing.T) {
		t.Run("empty storage", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return("", nil)

			results, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.Len(t, results, 1)

			result := results["default"]
			assert.True(t, result.Allowed)
			assert.Equal(t, quota.Limit, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("with existing state within window", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			window := FixedWindow{
				Name:  "default",
				Count: 5,
				Start: time.Now().Add(-30 * time.Second),
			}
			quotaStates := []FixedWindow{window}
			encodedState := encodeState(quotaStates)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return(encodedState, nil)

			results, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.Len(t, results, 1)

			result := results["default"]
			assert.True(t, result.Allowed)
			assert.Equal(t, quota.Limit-window.Count, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("with existing state window expired", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			window := FixedWindow{
				Count: 5,
				Start: time.Now().Add(-2 * time.Minute), // Expired
			}
			quotaStates := []FixedWindow{window}
			encodedState := encodeState(quotaStates)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return(encodedState, nil)

			results, err := Allow(ctx, storage, config, ReadOnly)
			assert.NoError(t, err)
			assert.Len(t, results, 1)

			result := results["default"]
			assert.True(t, result.Allowed)
			assert.Equal(t, quota.Limit, result.Remaining) // Reset to full capacity
			assert.False(t, result.stateUpdated)
		})

		t.Run("state retrieval error", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return("", errors.New("storage error"))

			_, err := Allow(ctx, storage, config, ReadOnly)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get fixed window state")
		})

		t.Run("state parsing error", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return("invalid-state", nil)

			_, err := Allow(ctx, storage, config, ReadOnly)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to parse fixed window state")
		})
	})

	t.Run("TryUpdate mode", func(t *testing.T) {
		t.Run("empty storage - allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return("", nil)
			storage.On("CheckAndSet", ctx, "test-key", "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil)

			results, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.Len(t, results, 1)

			result := results["default"]
			assert.True(t, result.Allowed)
			assert.Equal(t, quota.Limit-1, result.Remaining)
			assert.True(t, result.stateUpdated)
		})

		t.Run("with existing state - allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			window := FixedWindow{
				Name:  "default",
				Count: 5,
				Start: time.Now().Add(-30 * time.Second),
			}
			quotaStates := []FixedWindow{window}
			encodedState := encodeState(quotaStates)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return(encodedState, nil)
			storage.On("CheckAndSet", ctx, "test-key", encodedState, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil)

			results, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.Len(t, results, 1)

			result := results["default"]
			assert.True(t, result.Allowed)
			assert.True(t, result.stateUpdated)
		})

		t.Run("with existing state - not allowed", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			window := FixedWindow{
				Name:  "default",
				Count: quota.Limit, // At limit
				Start: time.Now().Add(-30 * time.Second),
			}
			quotaStates := []FixedWindow{window}
			encodedState := encodeState(quotaStates)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)
			storage.On("Get", ctx, "test-key").Return(encodedState, nil)

			results, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.Len(t, results, 1)

			result := results["default"]
			assert.False(t, result.Allowed)
			assert.Equal(t, 0, result.Remaining)
			assert.False(t, result.stateUpdated)
		})

		t.Run("concurrent access retry", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)

			// First Get returns empty
			storage.On("Get", ctx, "test-key").Return("", nil).Once()
			// First CheckAndSet fails for empty oldValue
			storage.On("CheckAndSet", ctx, "test-key", "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(false, nil).Once()

			// Second Get returns some state
			window := FixedWindow{Name: "default", Count: 5, Start: time.Now().Add(-30 * time.Second)}
			quotaStates := []FixedWindow{window}
			encodedState := encodeState(quotaStates)
			storage.On("Get", ctx, "test-key").Return(encodedState, nil).Once()
			// Second CheckAndSet succeeds
			storage.On("CheckAndSet", ctx, "test-key", encodedState, mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(true, nil).Once()

			results, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.Len(t, results, 1)

			result := results["default"]
			assert.True(t, result.Allowed)
			assert.True(t, result.stateUpdated)
		})

		t.Run("concurrent access max retries exceeded", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)

			storage.On("Get", ctx, "test-key").Return("", nil)
			storage.On("CheckAndSet", ctx, "test-key", "", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration")).Return(false, nil)

			_, err := Allow(ctx, storage, config, TryUpdate)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to update fixed window state after 3 attempts due to concurrent access")
		})

		t.Run("context canceled", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)
			canceledCtx, cancel := context.WithCancel(ctx)
			cancel()

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(quotas)
			config.On("GetMaxRetries").Return(maxRetries)

			_, err := Allow(canceledCtx, storage, config, TryUpdate)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "context canceled")
		})
	})

	t.Run("Multi-quota support", func(t *testing.T) {
		t.Run("all quotas allow", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			multiQuotas := []Quota{
				{Name: "requests", Limit: 100, Window: time.Minute},
				{Name: "writes", Limit: 10, Window: time.Minute},
			}

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(multiQuotas)
			config.On("GetMaxRetries").Return(maxRetries)

			// Setup storage responses for combined state
			storage.On("Get", ctx, "test-key").Return("", nil)
			storage.On("CheckAndSet", ctx, "test-key",
				"",                                   // oldVal
				mock.AnythingOfType("string"),        // newVal
				mock.AnythingOfType("time.Duration"), // expiration
			).
				Return(true, nil)

			results, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.Len(t, results, 2)

			for name, result := range results {
				assert.True(t, result.Allowed, "Quota %s should allow", name)
				assert.True(t, result.stateUpdated, "Quota %s should be updated", name)
			}
		})

		t.Run("one quota denies", func(t *testing.T) {
			storage := new(mockBackend)
			config := new(mockConfig)

			multiQuotas := []Quota{
				{Name: "requests", Limit: 100, Window: time.Minute},
				{Name: "writes", Limit: 1, Window: time.Minute}, // Will be at limit
			}

			// Setup combined state with writes at limit
			combinedState := []FixedWindow{
				{Name: "requests", Count: 0, Start: time.Now().Add(-30 * time.Second)}, // Not at limit
				{Name: "writes", Count: 1, Start: time.Now().Add(-30 * time.Second)},   // At limit (denied)
			}
			fullWindowState := encodeState(combinedState)

			config.On("GetKey").Return(key)
			config.On("GetQuotas").Return(multiQuotas)
			config.On("GetMaxRetries").Return(maxRetries)

			// Setup storage response for combined state
			storage.On("Get", ctx, "test-key").Return(fullWindowState, nil)
			// Set up CheckAndSet expectations - when one quota denies, others should not be updated
			storage.On("CheckAndSet", ctx, "test-key",
				mock.AnythingOfType("string"),        // oldVal
				mock.AnythingOfType("string"),        // newVal
				mock.AnythingOfType("time.Duration"), // expiration
			).
				Return(true, nil)

			results, err := Allow(ctx, storage, config, TryUpdate)
			assert.NoError(t, err)
			assert.Len(t, results, 2)

			// Requests should not be consumed because writes denied
			requestsResult := results["requests"]
			assert.True(t, requestsResult.Allowed)
			assert.False(t, requestsResult.stateUpdated) // Not consumed

			// Writes should be denied
			writesResult := results["writes"]
			assert.False(t, writesResult.Allowed)
		})
	})
}
