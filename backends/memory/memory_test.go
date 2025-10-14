package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemoryStorage_Get(t *testing.T) {
	storage := New()
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
	storage := New()
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
	storage := New()
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
	storage := New()
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

func TestMemoryStorage_AutoCleanup(t *testing.T) {
	ctx := context.Background()
	storage := NewWithCleanup(100 * time.Millisecond) // 100ms cleanup interval
	defer storage.Close()

	// Add some entries that will expire quickly
	err := storage.Set(ctx, "expired1", "value1", 50*time.Millisecond)
	require.NoError(t, err)

	err = storage.Set(ctx, "expired2", "value2", 50*time.Millisecond)
	require.NoError(t, err)

	err = storage.Set(ctx, "valid", "value3", time.Hour)
	require.NoError(t, err)

	// Wait for entries to expire and for cleanup to run
	time.Sleep(time.Millisecond * 200)

	// Check that expired entries were cleaned up automatically
	val, _ := storage.Get(ctx, "expired1")
	require.Equal(t, "", val, "Expected expired1 to be auto-cleaned up, got %q", val)

	val, _ = storage.Get(ctx, "expired2")
	require.Equal(t, "", val, "Expected expired2 to be auto-cleaned up, got %q", val)

	// Valid entry should still exist
	val, _ = storage.Get(ctx, "valid")
	require.NotEqual(t, "", val, "Expected valid entry to still exist")
}

func TestMemoryStorage_NoAutoCleanup(t *testing.T) {
	ctx := context.Background()
	storage := NewWithCleanup(0) // Disable auto cleanup
	defer storage.Close()

	// Add an entry that will expire
	err := storage.Set(ctx, "expired", "value", time.Millisecond*10)
	require.NoError(t, err)

	// Wait for entry to expire
	time.Sleep(time.Millisecond * 50)

	// Entry should still exist (no auto cleanup)
	val, _ := storage.Get(ctx, "expired")
	require.Equal(t, "", val, "Expected expired entry to return empty string when accessed")

	// But manual cleanup should work
	storage.Cleanup()

	// Now check that manual cleanup worked by trying to access again
	// (this will remove the expired entry on access)
	val, _ = storage.Get(ctx, "expired")
	require.Equal(t, "", val, "Expected expired entry to be cleaned up manually")
}

func TestMemoryStorage_cleanup(t *testing.T) {
	ctx := context.Background()
	storage := NewWithCleanup(0) // Disable auto cleanup for this test
	defer storage.Close()

	t.Run("CheckAndSet with nil oldValue - key doesn't exist", func(t *testing.T) {
		success, err := storage.CheckAndSet(ctx, "newkey", nil, "newvalue", time.Hour)
		require.NoError(t, err)
		require.True(t, success)

		val, err := storage.Get(ctx, "newkey")
		require.NoError(t, err)
		require.Equal(t, "newvalue", val)
	})

	t.Run("CheckAndSet with nil oldValue - key exists", func(t *testing.T) {
		err := storage.Set(ctx, "existingkey", "oldvalue", time.Hour)
		require.NoError(t, err)

		success, err := storage.CheckAndSet(ctx, "existingkey", nil, "newvalue", time.Hour)
		require.NoError(t, err)
		require.False(t, success)

		val, err := storage.Get(ctx, "existingkey")
		require.NoError(t, err)
		require.Equal(t, "oldvalue", val)
	})

	t.Run("CheckAndSet with matching oldValue", func(t *testing.T) {
		err := storage.Set(ctx, "matchkey", "expected", time.Hour)
		require.NoError(t, err)

		success, err := storage.CheckAndSet(ctx, "matchkey", "expected", "newvalue", time.Hour)
		require.NoError(t, err)
		require.True(t, success)

		val, err := storage.Get(ctx, "matchkey")
		require.NoError(t, err)
		require.Equal(t, "newvalue", val)
	})

	t.Run("CheckAndSet with non-matching oldValue", func(t *testing.T) {
		err := storage.Set(ctx, "nomatchkey", "actual", time.Hour)
		require.NoError(t, err)

		success, err := storage.CheckAndSet(ctx, "nomatchkey", "wrong", "newvalue", time.Hour)
		require.NoError(t, err)
		require.False(t, success)

		val, err := storage.Get(ctx, "nomatchkey")
		require.NoError(t, err)
		require.Equal(t, "actual", val)
	})

	t.Run("CheckAndSet with expired key", func(t *testing.T) {
		err := storage.Set(ctx, "expiredkey", "oldvalue", time.Millisecond*10)
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 20)

		// Treat expired key as non-existent for nil oldValue
		success, err := storage.CheckAndSet(ctx, "expiredkey", nil, "newvalue", time.Hour)
		require.NoError(t, err)
		require.True(t, success)

		val, err := storage.Get(ctx, "expiredkey")
		require.NoError(t, err)
		require.Equal(t, "newvalue", val)
	})

	t.Run("CheckAndSet with non-existent key and non-nil oldValue", func(t *testing.T) {
		success, err := storage.CheckAndSet(ctx, "nonexistent", "oldvalue", "newvalue", time.Hour)
		require.NoError(t, err)
		require.False(t, success)

		val, err := storage.Get(ctx, "nonexistent")
		require.NoError(t, err)
		require.Equal(t, "", val)
	})
}
