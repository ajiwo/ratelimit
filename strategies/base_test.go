package strategies

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	_ "github.com/ajiwo/ratelimit/backends/memory"
	"github.com/stretchr/testify/require"
)

func testCreateMemoryStorage(t *testing.T) backends.Backend {
	backend, err := backends.Create("memory", nil)
	require.NoError(t, err, "Failed to create memory storage")
	return backend
}

func TestLockCleanup(t *testing.T) {
	// Create a memory backend for testing
	backend, err := backends.Create("memory", nil)
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}

	// Test fixed window strategy cleanup
	fixedStrategy := NewFixedWindow(backend)

	// Create some locks
	keys := []string{"key1", "key2", "key3"}
	for _, key := range keys {
		lock := fixedStrategy.getLock(key)
		if lock == nil {
			t.Errorf("Expected lock for key %s", key)
		}
	}

	// Verify locks exist
	count := 0
	fixedStrategy.mu.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != len(keys) {
		t.Errorf("Expected %d locks, got %d", len(keys), count)
	}

	// Cleanup locks with max age of 0 (should remove all)
	fixedStrategy.Cleanup(0)

	// Verify locks are cleaned up
	count = 0
	fixedStrategy.mu.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("Expected 0 locks after cleanup, got %d", count)
	}

	// Test token bucket strategy cleanup
	tokenStrategy := NewTokenBucket(backend)

	// Create some locks
	for _, key := range keys {
		lock := tokenStrategy.getLock(key)
		if lock == nil {
			t.Errorf("Expected lock for key %s", key)
		}
	}

	// Verify locks exist
	count = 0
	tokenStrategy.mu.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != len(keys) {
		t.Errorf("Expected %d locks, got %d", len(keys), count)
	}

	// Cleanup locks
	tokenStrategy.Cleanup(0)

	// Verify locks are cleaned up
	count = 0
	tokenStrategy.mu.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("Expected 0 locks after cleanup, got %d", count)
	}

	// Test leaky bucket strategy cleanup
	leakyStrategy := NewLeakyBucket(backend)

	// Create some locks
	for _, key := range keys {
		lock := leakyStrategy.getLock(key)
		if lock == nil {
			t.Errorf("Expected lock for key %s", key)
		}
	}

	// Verify locks exist
	count = 0
	leakyStrategy.mu.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != len(keys) {
		t.Errorf("Expected %d locks, got %d", len(keys), count)
	}

	// Cleanup locks
	leakyStrategy.Cleanup(0)

	// Verify locks are cleaned up
	count = 0
	leakyStrategy.mu.Range(func(key, value any) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("Expected 0 locks after cleanup, got %d", count)
	}
}

func TestLockCleanupWithMaxAge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		backend, err := backends.Create("memory", nil)
		if err != nil {
			t.Fatalf("Failed to create memory backend: %v", err)
		}
		strategy := NewFixedWindow(backend)

		// Create locks
		strategy.getLock("key1")
		strategy.getLock("key2")

		// Wait a bit
		time.Sleep(10 * time.Millisecond)
		synctest.Wait()

		// Create another lock
		strategy.getLock("key3")

		// Cleanup with max age less than the wait time
		strategy.Cleanup(5 * time.Millisecond)

		// Should have removed key1 and key2, but kept key3
		count := 0
		hasKey3 := false
		strategy.mu.Range(func(key, value any) bool {
			count++
			if key.(string) == "key3" {
				hasKey3 = true
			}
			return true
		})

		if count != 1 {
			t.Errorf("Expected 1 lock after cleanup, got %d", count)
		}
		if !hasKey3 {
			t.Errorf("Expected key3 to still exist after cleanup")
		}
	})
}
