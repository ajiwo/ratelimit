package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setupPostgresTest(t *testing.T) (*PostgresStorage, func()) {
	t.Helper()

	postgresConn := os.Getenv("TEST_POSTGRES_DSN")
	if postgresConn == "" {
		postgresConn = "postgres://postgres:postgres@localhost:5432/ratelimit_test?sslmode=disable"
	}

	storage, err := NewPostgresStorage(PostgresConfig{
		ConnString: postgresConn,
		MaxConns:   5,
		MinConns:   1,
	})

	if err != nil {
		return nil, func() {}
	}

	teardown := func() {
		ctx := context.Background()
		_, _ = storage.GetPool().Exec(ctx, `TRUNCATE TABLE ratelimit_kv`)
		_ = storage.Close()
	}

	return storage, teardown
}

func TestPostgresStorage_Get(t *testing.T) {
	ctx := t.Context()
	storage, teardown := setupPostgresTest(t)
	defer teardown()

	if storage == nil {
		t.Skip("PostgreSQL not available, skipping tests")
	}

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

func TestPostgresStorage_Set(t *testing.T) {
	ctx := t.Context()
	storage, teardown := setupPostgresTest(t)
	defer teardown()
	if storage == nil {
		t.Skip("PostgreSQL not available, skipping tests")
	}

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

		val, err := storage.Get(ctx, "zeroexp")
		require.NoError(t, err)
		require.Equal(t, "value", val)
	})

	t.Run("Update existing key", func(t *testing.T) {
		err := storage.Set(ctx, "updatekey", "oldvalue", time.Hour)
		require.NoError(t, err)

		err = storage.Set(ctx, "updatekey", "newvalue", time.Hour)
		require.NoError(t, err)

		val, err := storage.Get(ctx, "updatekey")
		require.NoError(t, err)
		require.Equal(t, "newvalue", val)
	})
}

func TestPostgresStorage_Delete(t *testing.T) {
	ctx := t.Context()
	storage, teardown := setupPostgresTest(t)
	defer teardown()

	if storage == nil {
		t.Skip("PostgreSQL not available, skipping tests")
	}

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

func TestPostgresStorage_ConcurrentAccess(t *testing.T) {
	ctx := t.Context()
	storage, teardown := setupPostgresTest(t)
	defer teardown()

	if storage == nil {
		t.Skip("PostgreSQL not available, skipping tests")
	}

	const numGoroutines = 10
	const numOperations = 50

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)

	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

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

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent access error: %v", err)
		}
	}
}

func TestPostgresStorage_Close(t *testing.T) {
	postgresConn := os.Getenv("TEST_POSTGRES_DSN")
	if postgresConn == "" {
		postgresConn = "postgres://postgres:postgres@localhost:5432/ratelimit_test?sslmode=disable"
	}

	storage, err := NewPostgresStorage(PostgresConfig{
		ConnString: postgresConn,
		MaxConns:   5,
		MinConns:   1,
	})

	if err != nil {
		t.Skipf("PostgreSQL not available, skipping Close test: %v", err)
	}

	ctx := t.Context()

	err = storage.Set(ctx, "test_close_key", "test_value", time.Hour)
	require.NoError(t, err)

	val, err := storage.Get(ctx, "test_close_key")
	require.NoError(t, err)
	require.Equal(t, "test_value", val)

	err = storage.Close()
	require.NoError(t, err)

	_, err = storage.Get(ctx, "test_close_key")
	require.Error(t, err, "Expected error after closing connection")
}
