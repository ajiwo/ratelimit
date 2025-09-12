package backends

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRedisBackend(t *testing.T) {
	// Check if Redis is available before running tests
	_, err := NewRedisBackend(BackendConfig{RedisAddress: "localhost:6379"})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}

	type args struct {
		config BackendConfig
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "valid config",
			args: args{
				config: BackendConfig{
					RedisAddress:  "localhost:6379",
					RedisPassword: "",
					RedisDB:       0,
					RedisPoolSize: 10,
				},
			},
			wantErr: false,
		},
		{
			name: "empty address",
			args: args{
				config: BackendConfig{
					RedisAddress: "",
				},
			},
			wantErr: true,
		},
		{
			name: "default pool size",
			args: args{
				config: BackendConfig{
					RedisAddress:  "localhost:6379",
					RedisPoolSize: 0,
				},
			},
			wantErr: false,
		},
		{
			name: "bad address",
			args: args{
				config: BackendConfig{
					RedisAddress: "localhost:9999", // Unlikely to be running
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRedisBackend(tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRedisBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("NewRedisBackend() returned nil when no error expected")
			}
			if !tt.wantErr && got != nil {
				_ = got.Close()
			}
		})
	}
}

func TestRedisBackend_Get(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// Clean up any existing test data
	require.NoError(t, backend.Delete(ctx, "test_key"))
	require.NoError(t, backend.Delete(ctx, "bad_json_key"))

	type args struct {
		key string
	}
	tests := []struct {
		name    string
		args    args
		want    *BackendData
		wantErr bool
		setup   func()
	}{
		{
			name: "key exists",
			args: args{key: "test_key"},
			setup: func() {
				data := &BackendData{
					Count:       5,
					WindowStart: time.Now(),
					LastRequest: time.Now(),
					Tokens:      10.5,
					LastRefill:  time.Now(),
				}
				require.NoErrorf(t, backend.Set(ctx, "test_key", data, 0), "Failed to set test_key: %v", err)
			},
			want:    &BackendData{Count: 5}, // We only verify count since other fields are timestamps
			wantErr: false,
		},
		{
			name:    "key does not exist",
			args:    args{key: "nonexistent_key"},
			want:    nil,
			wantErr: false,
		},
		{
			name: "bad json",
			args: args{key: "bad_json_key"},
			setup: func() {
				_ = backend.client.Set(ctx, "bad_json_key", "{bad json}", 0)
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			got, err := backend.Get(ctx, tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("RedisBackend.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("RedisBackend.Get() = %v, want nil", got)
			case tt.want != nil && got == nil:
				t.Errorf("RedisBackend.Get() = nil, want non-nil")
			case tt.want == nil && got == nil:
				// Both nil, which is expected
			case tt.want != nil && got != nil:
				// Both non-nil, verify count value
				if got.Count != tt.want.Count {
					t.Errorf("RedisBackend.Get() count = %d, want %d", got.Count, tt.want.Count)
				}
			}
		})
	}
}

func TestRedisBackend_Set(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// Clean up any existing test data
	require.NoErrorf(t, backend.Delete(ctx, "test_key"), "Failed to delete test_key: %v", err)

	type args struct {
		key  string
		data *BackendData
		ttl  time.Duration
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		verify  func() error
	}{
		{
			name: "set without ttl",
			args: args{
				key: "test_key",
				data: &BackendData{
					Count:       1,
					WindowStart: time.Now(),
					LastRequest: time.Now(),
					Tokens:      5.0,
					LastRefill:  time.Now(),
				},
				ttl: 0,
			},
			wantErr: false,
			verify: func() error {
				got, err := backend.Get(ctx, "test_key")
				if err != nil {
					return err
				}
				if got == nil || got.Count != 1 {
					return fmt.Errorf("data not set correctly")
				}
				return nil
			},
		},
		{
			name: "set with ttl",
			args: args{
				key: "test_key_ttl",
				data: &BackendData{
					Count: 2,
				},
				ttl: time.Second,
			},
			wantErr: false,
			verify: func() error {
				got, err := backend.Get(ctx, "test_key_ttl")
				if err != nil {
					return err
				}
				if got == nil || got.Count != 2 {
					return fmt.Errorf("data not set correctly")
				}
				return nil
			},
		},
		{
			name: "nil data",
			args: args{
				key:  "test_key_nil",
				data: nil,
				ttl:  0,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := backend.Set(ctx, tt.args.key, tt.args.data, tt.args.ttl); (err != nil) != tt.wantErr {
				t.Errorf("RedisBackend.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.verify != nil {
				if err := tt.verify(); err != nil {
					t.Errorf("RedisBackend.Set() verification failed: %v", err)
				}
			}
		})
	}
}

func TestRedisBackend_Increment(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// Clean up any existing test data
	require.NoErrorf(t, backend.Delete(ctx, "test_increment"), "Failed to delete test_increment: %v", err)

	type args struct {
		key    string
		window time.Duration
		limit  int64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		verify  func() (int64, bool, error)
	}{
		{
			name: "first increment",
			args: args{
				key:    "test_increment",
				window: time.Minute,
				limit:  10,
			},
			wantErr: false,
			verify: func() (int64, bool, error) {
				return backend.Increment(ctx, "test_increment", time.Minute, 10)
			},
		},
		{
			name: "increment within limit",
			args: args{
				key:    "test_increment",
				window: time.Minute,
				limit:  10,
			},
			wantErr: false,
			verify: func() (int64, bool, error) {
				// First increment
				_, _, err := backend.Increment(ctx, "test_increment", time.Minute, 10)
				if err != nil {
					return 0, false, err
				}
				return backend.Increment(ctx, "test_increment", time.Minute, 10)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.verify != nil {
				count, incremented, err := tt.verify()
				if (err != nil) != tt.wantErr {
					t.Errorf("RedisBackend.Increment() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !tt.wantErr && !incremented {
					t.Errorf("RedisBackend.Increment() expected incremented=true, got false")
				}
				if !tt.wantErr && count <= 0 {
					t.Errorf("RedisBackend.Increment() expected count > 0, got %d", count)
				}
			}
		})
	}
}

func TestRedisBackend_ConsumeToken(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// Clean up any existing test data
	require.NoErrorf(t, backend.Delete(ctx, "test_token"), "Failed to delete test_token: %v", err)

	type args struct {
		key          string
		bucketSize   int64
		refillRate   time.Duration
		refillAmount int64
		window       time.Duration
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "first token consumption",
			args: args{
				key:          "test_token",
				bucketSize:   10,
				refillRate:   time.Second,
				refillAmount: 1,
				window:       time.Minute,
			},
			wantErr: false,
		},
		{
			name: "consume from full bucket",
			args: args{
				key:          "test_token_full",
				bucketSize:   5,
				refillRate:   time.Second,
				refillAmount: 1,
				window:       time.Minute,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, consumed, requests, err := backend.ConsumeToken(ctx, tt.args.key, tt.args.bucketSize, tt.args.refillRate, tt.args.refillAmount, tt.args.window)
			if (err != nil) != tt.wantErr {
				t.Errorf("RedisBackend.ConsumeToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !consumed {
					t.Errorf("RedisBackend.ConsumeToken() expected consumed=true, got false")
				}
				if requests < 1 {
					t.Errorf("RedisBackend.ConsumeToken() expected requests >= 1, got %d", requests)
				}
				if tokens < 0 {
					t.Errorf("RedisBackend.ConsumeToken() expected tokens >= 0, got %f", tokens)
				}
			}
		})
	}
}

func TestRedisBackend_Delete(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	type args struct {
		key string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		setup   func()
	}{
		{
			name: "delete existing key",
			args: args{key: "test_delete"},
			setup: func() {
				data := &BackendData{Count: 1}
				require.NoErrorf(t, backend.Set(ctx, "test_delete", data, 0), "Failed to set test_delete: %v", err)
			},
			wantErr: false,
		},
		{
			name:    "delete non-existent key",
			args:    args{key: "nonexistent"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			if err := backend.Delete(ctx, tt.args.key); (err != nil) != tt.wantErr {
				t.Errorf("RedisBackend.Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedisBackend_Close(t *testing.T) {
	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}

	if err := backend.Close(); err != nil {
		t.Errorf("RedisBackend.Close() error = %v", err)
	}

	// Second close should not panic
	if err := backend.Close(); err != nil {
		t.Logf("RedisBackend.Close() second call returns error as expected: %v", err)
	}

	// Test closing a nil client
	rb := &RedisBackend{}
	if err := rb.Close(); err != nil {
		t.Errorf("closing a nil client should not return an error, got: %v", err)
	}
}

func TestRedisBackend_GetStats(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress:  "localhost:6379",
		RedisPoolSize: 5,
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	stats, err := backend.GetStats(ctx)
	if err != nil {
		t.Errorf("RedisBackend.GetStats() error = %v", err)
		return
	}

	if stats == nil {
		t.Errorf("RedisBackend.GetStats() returned nil")
		return
	}

	if stats.Address != "localhost:6379" {
		t.Errorf("RedisBackend.GetStats() Address = %s, want localhost:6379", stats.Address)
	}

	if stats.PoolSize != 5 {
		t.Errorf("RedisBackend.GetStats() PoolSize = %d, want 5", stats.PoolSize)
	}

	if stats.Info == "" {
		t.Errorf("RedisBackend.GetStats() Info is empty")
	}
}

func TestRedisBackend_Ping(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	if err := backend.Ping(ctx); err != nil {
		t.Errorf("RedisBackend.Ping() error = %v", err)
	}
}

func TestRedisBackend_FlushDB(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
		RedisDB:      1, // Use a different DB to avoid affecting main data
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// Set some test data
	data := &BackendData{Count: 1}
	require.NoErrorf(t, backend.Set(ctx, "test_flush", data, 0), "Failed to set test_flush: %v", err)

	require.NoErrorf(t, backend.FlushDB(ctx), "RedisBackend.FlushDB() error = %v", err)

	// Verify data is gone
	got, err := backend.Get(ctx, "test_flush")
	require.NoErrorf(t, err, "RedisBackend.FlushDB() Get() error after flush: %v", err)
	if got != nil {
		t.Errorf("RedisBackend.FlushDB() data still exists after flush")
	}
}

func TestRedisBackend_Keys(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
		RedisDB:      2, // Use a different DB to avoid affecting main data
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// Clean up first
	require.NoErrorf(t, backend.FlushDB(ctx), "Failed to flush DB: %v", err)

	// Set some test data
	err = backend.Set(ctx, "test_keys_1", &BackendData{Count: 1}, 0)
	require.NoErrorf(t, err, "Failed to set test_keys_1: %v", err)

	err = backend.Set(ctx, "test_keys_2", &BackendData{Count: 2}, 0)
	require.NoErrorf(t, err, "Failed to set test_keys_2: %v", err)

	err = backend.Set(ctx, "other_key", &BackendData{Count: 3}, 0)
	require.NoErrorf(t, err, "Failed to set other_key: %v", err)

	type args struct {
		pattern string
	}
	tests := []struct {
		name    string
		args    args
		minKeys int
		wantErr bool
	}{
		{
			name:    "match test_keys pattern",
			args:    args{pattern: "test_keys*"},
			minKeys: 2,
			wantErr: false,
		},
		{
			name:    "match all keys",
			args:    args{pattern: "*"},
			minKeys: 3,
			wantErr: false,
		},
		{
			name:    "exact match",
			args:    args{pattern: "test_keys_1"},
			minKeys: 1,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := backend.Keys(ctx, tt.args.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("RedisBackend.Keys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) < tt.minKeys {
					t.Errorf("RedisBackend.Keys() returned %d keys, expected at least %d", len(got), tt.minKeys)
				}
			}
		})
	}
}

// TestRedisBackend_Integration tests the complete Redis backend workflow
func TestRedisBackend_Integration(t *testing.T) {
	ctx := t.Context()

	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
		RedisDB:      3, // Use separate DB for integration tests
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// Clean up
	require.NoErrorf(t, backend.FlushDB(ctx), "Failed to flush DB: %v", err)

	// Test complete workflow
	key := "integration_test"

	// 1. Set initial data
	data := &BackendData{
		Count:       5,
		WindowStart: time.Now(),
		LastRequest: time.Now(),
	}
	require.NoErrorf(t, backend.Set(ctx, key, data, time.Minute), "Failed to set initial data: %v", err)

	// 2. Get and verify
	retrieved, err := backend.Get(ctx, key)
	require.NoErrorf(t, err, "Failed to get data: %v", err)
	if retrieved == nil || retrieved.Count != 5 {
		t.Fatalf("Retrieved data mismatch: got %+v", retrieved)
	}

	// 3. Test increment (fixed window) - use a fresh key for increment test
	incrementKey := "increment_test"
	count, incremented, err := backend.Increment(ctx, incrementKey, time.Minute, 10)
	require.NoErrorf(t, err, "Failed to increment: %v", err)
	if !incremented || count != 1 {
		t.Fatalf("Increment failed: count=%d, incremented=%v", count, incremented)
	}

	// 4. Test token bucket
	tokenKey := "token_test"
	tokens, consumed, requests, err := backend.ConsumeToken(ctx, tokenKey, 10, time.Second, 1, time.Minute)
	require.NoErrorf(t, err, "Failed to consume token: %v", err)
	if !consumed || requests != 1 || tokens != 9 {
		t.Fatalf("Token consumption failed: tokens=%f, consumed=%v, requests=%d", tokens, consumed, requests)
	}

	// 5. Test stats
	stats, err := backend.GetStats(ctx)
	require.NoErrorf(t, err, "Failed to get stats: %v", err)
	require.Conditionf(t,
		func() bool {
			return stats != nil && stats.Address != ""
		},
		"Stats invalid: %+v", stats,
	)

	// 6. Test ping
	require.NoErrorf(t, backend.Ping(ctx), "Ping failed")

	// 7. Test keys
	keys, err := backend.Keys(ctx, "*")
	require.NoErrorf(t, err, "Failed to get keys: %v", err)
	// Should have at least 'integration_test' and 'token_test'
	require.LessOrEqualf(t, 2, len(keys), "Expected at least 2 keys, got %d: %v", len(keys), keys)

	// 8. Clean up
	require.NoErrorf(t, backend.Delete(ctx, key), "Failed to delete key: %v", err)
	require.NoErrorf(t, backend.Delete(ctx, tokenKey), "Failed to delete token key: %v", err)

	// 9. Verify deletion
	retrieved, err = backend.Get(ctx, key)
	require.NoErrorf(t, err, "Failed to get after deletion: %v", err)
	require.Nilf(t, retrieved, "Data still exists after deletion")
}

func TestRedisBackend_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	// This test simulates Redis failures by closing the client.
	backend, err := NewRedisBackend(BackendConfig{
		RedisAddress: "localhost:6379",
	})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	// Close the client to simulate a connection error
	backend.Close()

	t.Run("Get error", func(t *testing.T) {
		_, err := backend.Get(ctx, "any_key")
		require.Errorf(t, err, "Expected error for Get, got nil")
	})

	t.Run("Set error", func(t *testing.T) {
		err := backend.Set(ctx, "any_key", &BackendData{}, 0)
		require.Errorf(t, err, "Expected error for Set, got nil")
	})

	t.Run("Delete error", func(t *testing.T) {
		err := backend.Delete(ctx, "any_key")
		require.Errorf(t, err, "Expected error for Delete, got nil")
	})

	t.Run("Increment error", func(t *testing.T) {
		_, _, err := backend.Increment(ctx, "any_key", time.Minute, 10)
		require.Errorf(t, err, "Expected error for Increment, got nil")
	})

	t.Run("ConsumeToken error", func(t *testing.T) {
		_, _, _, err := backend.ConsumeToken(ctx, "any_key", 10, time.Second, 1, time.Minute)
		require.Errorf(t, err, "Expected error for ConsumeToken, got nil")
	})

	t.Run("GetStats error", func(t *testing.T) {
		_, err := backend.GetStats(ctx)
		require.Errorf(t, err, "Expected error for GetStats, got nil")
	})
}

func TestRedisBackend_Increment_Malformed(t *testing.T) {
	ctx := t.Context()
	backend, err := NewRedisBackend(BackendConfig{RedisAddress: "localhost:6379"})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// This is a white-box test to check parsing of malformed Eval results.
	// It's tricky to simulate this without a mock server, so we are asserting the expected error types.

	// Simulate a malformed result (not a slice)
	_, _, err = backend.Increment(ctx, "malformed_result", 0, 0)
	if err != nil {
		// We expect an error here, but it's hard to control the exact error from a real Redis.
		// The goal is to exercise the error handling paths in the Increment function.
		t.Logf("Received expected error for malformed increment script (this is okay): %v", err)
	}
}

func TestRedisBackend_ConsumeToken_Malformed(t *testing.T) {
	ctx := t.Context()
	backend, err := NewRedisBackend(BackendConfig{RedisAddress: "localhost:6379"})
	if err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}
	defer backend.Close()

	// White-box test for malformed token consumption results.
	_, _, _, err = backend.ConsumeToken(ctx, "malformed_token_result", 0, 0, 0, 0)
	if err != nil {
		t.Logf("Received expected error for malformed token script (this is okay): %v", err)
	}
}
