package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewMemoryStorage(t *testing.T) {
	storage := NewMemoryStorage()
	require.NotNil(t, storage)
	require.NotNil(t, storage.values)
}

func TestMemoryStorage_Get(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	t.Run("Get non-existent key", func(t *testing.T) {
		val, err := storage.Get(ctx, "nonexistent")
		require.NoErrorf(t, err, "Expected no error, got %v", err)
		require.Equalf(t, "", val, "Expected empty string, got %q", val)
	})

	t.Run("Get existing string value", func(t *testing.T) {
		err := storage.Set(ctx, "testkey", "testvalue", time.Hour)
		require.NoErrorf(t, err, "Failed to set value: %v", err)

		val, err := storage.Get(ctx, "testkey")
		require.NoErrorf(t, err, "Expected no error, got %v", err)
		require.Equalf(t, "testvalue", val, "Expected %q, got %q", "testvalue", val)
	})

	t.Run("Get existing int value", func(t *testing.T) {
		err := storage.Set(ctx, "intkey", 42, time.Hour)
		require.NoError(t, err)

		val, err := storage.Get(ctx, "intkey")
		require.NoError(t, err)
		require.Equal(t, "42", val)
	})

	t.Run("Get expired value", func(t *testing.T) {
		err := storage.Set(ctx, "expiredkey", "expiredvalue", time.Millisecond*10)
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 20)

		val, err := storage.Get(ctx, "expiredkey")
		require.NoError(t, err)
		require.Equal(t, "", val)
	})
}

func TestMemoryStorage_Set(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := t.Context()

	t.Run("Set string value", func(t *testing.T) {
		err := storage.Set(ctx, "stringkey", "testvalue", time.Hour)
		require.NoError(t, err)

		val, err := storage.Get(ctx, "stringkey")
		require.NoError(t, err)
		require.Equal(t, "testvalue", val)
	})

	t.Run("Set int value", func(t *testing.T) {
		err := storage.Set(ctx, "intkey", 123, time.Hour)
		require.NoError(t, err)

		val, err := storage.Get(ctx, "intkey")
		require.NoError(t, err)
		require.Equal(t, "123", val)
	})

	t.Run("Set with zero expiration", func(t *testing.T) {
		err := storage.Set(ctx, "zeroexp", "value", 0)
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 10)
		val, err := storage.Get(ctx, "zeroexp")
		require.NoError(t, err)
		require.Equal(t, "", val)
	})
}

func TestMemoryStorage_Delete(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	t.Run("Delete existing key", func(t *testing.T) {
		err := storage.Set(ctx, "deletekey", "value", time.Hour)
		require.NoError(t, err)

		err = storage.Delete(ctx, "deletekey")
		require.NoError(t, err)

		val, err := storage.Get(ctx, "deletekey")
		require.NoError(t, err)
		require.Equal(t, "", val)
	})

	t.Run("Delete non-existent key", func(t *testing.T) {
		err := storage.Delete(ctx, "nonexistent")
		require.NoError(t, err)
	})
}

func TestMemoryStorage_ConcurrentAccess(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	const numGoroutines = 10
	const numOperations = 100

	done := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines*numOperations)

	for i := range numGoroutines {
		go func(id int) {
			defer func() { done <- true }()

			for j := range numOperations {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)

				err := storage.Set(ctx, key, value, time.Hour)
				if err != nil {
					errors <- err
					continue
				}

				retrieved, err := storage.Get(ctx, key)
				if err != nil {
					errors <- err
					continue
				}

				if retrieved != value {
					errors <- fmt.Errorf("expected %q, got %q", value, retrieved)
					continue
				}

				err = storage.Delete(ctx, key)
				if err != nil {
					errors <- err
					continue
				}
			}
		}(i)
	}

	for range numGoroutines {
		<-done
	}

	close(errors)
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent access error: %v", err)
		}
	}
}

func TestMemoryStorage_cleanup(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	t.Run("Cleanup removes expired entries", func(t *testing.T) {
		err := storage.Set(ctx, "expired1", "value1", time.Millisecond*10)
		require.NoError(t, err)

		err = storage.Set(ctx, "expired2", "value2", time.Millisecond*10)
		require.NoError(t, err)

		err = storage.Set(ctx, "valid", "value3", time.Hour)
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 20)

		storage.mu.Lock()
		storage.cleanup()
		storage.mu.Unlock()

		val, _ := storage.Get(ctx, "expired1")
		require.Equal(t, "", val, "Expected expired1 to be cleaned up, got %q", val)
		val, _ = storage.Get(ctx, "expired2")
		require.Equal(t, "", val, "Expected expired2 to be cleaned up, got %q", val)
		val, _ = storage.Get(ctx, "valid")
		require.Equal(t, "value3", val, "Expected valid to remain, got %q", val)
	})
}

func TestMemoryStorage_Close(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := t.Context()

	// Add some data to the storage
	err := storage.Set(ctx, "key1", "value1", time.Hour)
	require.NoError(t, err)
	err = storage.Set(ctx, "key2", "value2", time.Hour)
	require.NoError(t, err)

	// Verify data exists
	val, err := storage.Get(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, "value1", val)

	// Close the storage
	err = storage.Close()
	require.NoError(t, err)

	// Verify storage is effectively cleared (new operations should work but find no data)
	val, err = storage.Get(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, "", val)
}
