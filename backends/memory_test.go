package backends

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_copyMetadata(t *testing.T) {
	type args struct {
		metadata map[string]any
	}
	tests := []struct {
		name string
		args args
		want map[string]any
	}{
		{
			name: "nil metadata",
			args: args{metadata: nil},
			want: map[string]any{},
		},
		{
			name: "empty metadata",
			args: args{metadata: map[string]any{}},
			want: map[string]any{},
		},
		{
			name: "metadata with values",
			args: args{metadata: map[string]any{"key1": "value1", "key2": 42}},
			want: map[string]any{"key1": "value1", "key2": 42},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := copyMetadata(tt.args.metadata)
			if len(got) != len(tt.want) {
				t.Errorf("copyMetadata() length mismatch: got %d, want %d", len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("copyMetadata()[%s] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestNewMemoryBackend(t *testing.T) {
	type args struct {
		config BackendConfig
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		verify  func(*MemoryBackend) error
	}{
		{
			name:    "default config",
			args:    args{config: BackendConfig{}},
			wantErr: false,
			verify: func(backend *MemoryBackend) error {
				if backend.maxKeys != 10000 {
					return fmt.Errorf("expected maxKeys=10000, got %d", backend.maxKeys)
				}
				if backend.cleanupInterval != 5*time.Minute {
					return fmt.Errorf("expected cleanupInterval=5m, got %v", backend.cleanupInterval)
				}
				if len(backend.data) != 0 {
					return fmt.Errorf("expected empty data, got %d items", len(backend.data))
				}
				return nil
			},
		},
		{
			name: "custom config",
			args: args{
				config: BackendConfig{
					CleanupInterval: 10 * time.Second,
					MaxKeys:         500,
				},
			},
			wantErr: false,
			verify: func(backend *MemoryBackend) error {
				if backend.maxKeys != 500 {
					return fmt.Errorf("expected maxKeys=500, got %d", backend.maxKeys)
				}
				if backend.cleanupInterval != 10*time.Second {
					return fmt.Errorf("expected cleanupInterval=10s, got %v", backend.cleanupInterval)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewMemoryBackend(tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMemoryBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				defer got.Close()
				if tt.verify != nil {
					if err := tt.verify(got); err != nil {
						t.Errorf("NewMemoryBackend() verification failed: %v", err)
					}
				}
			}
		})
	}
}

func TestMemoryBackend_Get(t *testing.T) {
	ctx := t.Context()

	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Hour, // Long interval to avoid cleanup during tests
		MaxKeys:         1000,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}
	defer backend.Close()

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
					Metadata:    map[string]any{"test": "value"},
				}
				_ = backend.Set(ctx, "test_key", data, 0)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			got, err := backend.Get(ctx, tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("MemoryBackend.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			switch {
			case tt.want == nil && got != nil:
				t.Errorf("MemoryBackend.Get() = %v, want nil", got)
			case tt.want != nil && got == nil:
				t.Errorf("MemoryBackend.Get() = nil, want non-nil")
			case tt.want == nil && got == nil:
				// Both nil, which is expected
			case tt.want != nil && got != nil:
				// Both non-nil, verify count value
				if got.Count != tt.want.Count {
					t.Errorf("MemoryBackend.Get() count = %d, want %d", got.Count, tt.want.Count)
				}
			}
		})
	}
}

func TestMemoryBackend_Set(t *testing.T) {
	ctx := t.Context()

	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Hour,
		MaxKeys:         2, // Small limit to test capacity
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}
	defer backend.Close()

	type args struct {
		key  string
		data *BackendData
		ttl  time.Duration
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		setup   func()
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
		{
			name: "exceed capacity",
			args: args{
				key:  "test_key_3",
				data: &BackendData{Count: 1},
				ttl:  0,
			},
			wantErr: true,
			setup: func() {
				// Fill up the backend first
				_ = backend.Set(ctx, "key1", &BackendData{Count: 1}, 0)
				_ = backend.Set(ctx, "key2", &BackendData{Count: 1}, 0)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			if err := backend.Set(ctx, tt.args.key, tt.args.data, tt.args.ttl); (err != nil) != tt.wantErr {
				t.Errorf("MemoryBackend.Set() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.verify != nil {
				if err := tt.verify(); err != nil {
					t.Errorf("MemoryBackend.Set() verification failed: %v", err)
				}
			}
		})
	}
}

func TestMemoryBackend_Increment(t *testing.T) {
	ctx := t.Context()

	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Hour,
		MaxKeys:         1000,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}
	defer backend.Close()

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
				key:    "test_increment_within",
				window: time.Minute,
				limit:  5,
			},
			wantErr: false,
			verify: func() (int64, bool, error) {
				// First increment
				_, _, err := backend.Increment(ctx, "test_increment_within", time.Minute, 5)
				require.NoError(t, err)
				return backend.Increment(ctx, "test_increment_within", time.Minute, 5)
			},
		},
		{
			name: "increment at limit",
			args: args{
				key:    "test_increment_limit",
				window: time.Minute,
				limit:  2,
			},
			wantErr: false,
			verify: func() (int64, bool, error) {
				// Fill up to limit
				_, _, err := backend.Increment(ctx, "test_increment_limit", time.Minute, 2)
				require.NoError(t, err)
				_, _, err = backend.Increment(ctx, "test_increment_limit", time.Minute, 2)
				require.NoError(t, err)
				// This should fail
				return backend.Increment(ctx, "test_increment_limit", time.Minute, 2)
			},
		},
		{
			name: "window reset",
			args: args{
				key:    "test_window_reset",
				window: time.Millisecond, // Very short window
				limit:  5,
			},
			wantErr: false,
			verify: func() (int64, bool, error) {
				// First increment
				_, _, err := backend.Increment(ctx, "test_window_reset", time.Millisecond, 5)
				require.NoError(t, err)
				// Wait for window to reset
				time.Sleep(2 * time.Millisecond)
				// Should reset window and allow increment
				return backend.Increment(ctx, "test_window_reset", time.Millisecond, 5)
			},
		},
		{
			name: "exceed capacity on new key",
			args: args{
				key:    "test_increment_capacity",
				window: time.Minute,
				limit:  10,
			},
			wantErr: true,
			verify: func() (int64, bool, error) {
				// Create a backend with small capacity
				smallBackend, _ := NewMemoryBackend(BackendConfig{MaxKeys: 1})
				defer smallBackend.Close()
				// Fill it
				_, _, err := smallBackend.Increment(ctx, "some_key", time.Minute, 1)
				require.NoError(t, err)
				// This should fail
				return smallBackend.Increment(ctx, "test_increment_capacity", time.Minute, 10)
			},
		},
		{
			name: "zero limit on new key",
			args: args{
				key:    "test_increment_zero_limit",
				window: time.Minute,
				limit:  0,
			},
			wantErr: false,
			verify: func() (int64, bool, error) {
				count, allowed, err := backend.Increment(ctx, "test_increment_zero_limit", time.Minute, 0)
				if err != nil {
					return 0, false, err
				}
				if allowed {
					return 0, false, fmt.Errorf("increment should not be allowed for limit 0")
				}
				if count != 0 {
					return 0, false, fmt.Errorf("count should be 0 for limit 0, got %d", count)
				}
				return count, allowed, err
			},
		},
		{
			name: "zero limit on window reset",
			args: args{
				key:    "test_increment_zero_limit_reset",
				window: time.Millisecond,
				limit:  0,
			},
			wantErr: false,
			verify: func() (int64, bool, error) {
				// Set some initial data
				require.NoError(t, backend.Set(ctx, "test_increment_zero_limit_reset", &BackendData{Count: 5, WindowStart: time.Now().Add(-time.Second)}, 0))
				time.Sleep(2 * time.Millisecond) // Ensure window is expired
				count, allowed, err := backend.Increment(ctx, "test_increment_zero_limit_reset", time.Millisecond, 0)
				if err != nil {
					return 0, false, err
				}
				if allowed {
					return 0, false, fmt.Errorf("increment should not be allowed for limit 0 on reset")
				}
				if count != 0 {
					return 0, false, fmt.Errorf("count should be 0 for limit 0 on reset, got %d", count)
				}
				return count, allowed, err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.verify != nil {
				_, _, err := tt.verify()
				if (err != nil) != tt.wantErr {
					t.Errorf("MemoryBackend.Increment() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			}
		})
	}
}

func TestMemoryBackend_ConsumeToken(t *testing.T) {
	ctx := t.Context()

	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Hour,
		MaxKeys:         1000,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}
	defer backend.Close()

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
		verify  func() (float64, bool, int64, error)
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
			verify: func() (float64, bool, int64, error) {
				return backend.ConsumeToken(ctx, "test_token", 10, time.Second, 1, time.Minute)
			},
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
			verify: func() (float64, bool, int64, error) {
				// First consumption
				_, _, _, err := backend.ConsumeToken(ctx, "test_token_full", 5, time.Second, 1, time.Minute)
				require.NoError(t, err)
				return backend.ConsumeToken(ctx, "test_token_full", 5, time.Second, 1, time.Minute)
			},
		},
		{
			name: "bucket exhausted",
			args: args{
				key:          "test_token_empty",
				bucketSize:   2,
				refillRate:   time.Second,
				refillAmount: 1,
				window:       time.Minute,
			},
			wantErr: false,
			verify: func() (float64, bool, int64, error) {
				// Exhaust the bucket
				_, _, _, err := backend.ConsumeToken(ctx, "test_token_empty", 2, time.Second, 1, time.Minute)
				require.NoError(t, err)
				_, _, _, err = backend.ConsumeToken(ctx, "test_token_empty", 2, time.Second, 1, time.Minute)
				require.NoError(t, err)
				// This should fail
				return backend.ConsumeToken(ctx, "test_token_empty", 2, time.Second, 1, time.Minute)
			},
		},
		{
			name: "token refill",
			args: args{
				key:          "test_token_refill",
				bucketSize:   5,
				refillRate:   time.Millisecond, // Fast refill for testing
				refillAmount: 1,
				window:       time.Minute,
			},
			wantErr: false,
			verify: func() (float64, bool, int64, error) {
				// Exhaust the bucket
				_, _, _, err1 := backend.ConsumeToken(ctx, "test_token_refill", 5, time.Millisecond, 1, time.Minute)
				require.NoError(t, err1)
				_, _, _, err2 := backend.ConsumeToken(ctx, "test_token_refill", 5, time.Millisecond, 1, time.Minute)
				require.NoError(t, err2)
				_, _, _, err3 := backend.ConsumeToken(ctx, "test_token_refill", 5, time.Millisecond, 1, time.Minute)
				require.NoError(t, err3)
				_, _, _, err4 := backend.ConsumeToken(ctx, "test_token_refill", 5, time.Millisecond, 1, time.Minute)
				require.NoError(t, err4)
				_, _, _, err5 := backend.ConsumeToken(ctx, "test_token_refill", 5, time.Millisecond, 1, time.Minute)
				require.NoError(t, err5)
				// Wait for refill
				time.Sleep(2 * time.Millisecond)
				// Should be able to consume again
				return backend.ConsumeToken(ctx, "test_token_refill", 5, time.Millisecond, 1, time.Minute)
			},
		},
		{
			name: "exceed capacity on new key",
			args: args{
				key:          "test_token_capacity",
				bucketSize:   10,
				refillRate:   time.Second,
				refillAmount: 1,
				window:       time.Minute,
			},
			wantErr: true,
			verify: func() (float64, bool, int64, error) {
				smallBackend, _ := NewMemoryBackend(BackendConfig{MaxKeys: 1})
				defer smallBackend.Close()
				_, _, _, err := smallBackend.ConsumeToken(ctx, "some_key", 10, time.Second, 1, time.Minute)
				require.NoError(t, err)
				return smallBackend.ConsumeToken(ctx, "test_token_capacity", 10, time.Second, 1, time.Minute)
			},
		},
		{
			name: "zero bucket size on new key",
			args: args{
				key:          "test_token_zero_bucket",
				bucketSize:   0,
				refillRate:   time.Second,
				refillAmount: 1,
				window:       time.Minute,
			},
			wantErr: false,
			verify: func() (float64, bool, int64, error) {
				tokens, allowed, count, err := backend.ConsumeToken(ctx, "test_token_zero_bucket", 0, time.Second, 1, time.Minute)
				if err != nil {
					return 0, false, 0, err
				}
				if allowed {
					return 0, false, 0, fmt.Errorf("token consumption should not be allowed for bucket size 0")
				}
				return tokens, allowed, count, err
			},
		},
		{
			name: "zero refill rate - not enough tokens",
			args: args{
				key:          "test_token_zero_refill_fail",
				bucketSize:   10,
				refillRate:   0,
				refillAmount: 0,
				window:       time.Minute,
			},
			wantErr: false,
			verify: func() (float64, bool, int64, error) {
				// Set state with zero tokens
				require.NoError(t, backend.Set(ctx, "test_token_zero_refill_fail", &BackendData{Tokens: 0}, 0))
				tokens, allowed, count, err := backend.ConsumeToken(ctx, "test_token_zero_refill_fail", 10, 0, 0, time.Minute)
				if err != nil {
					return 0, false, 0, err
				}
				if allowed {
					return 0, false, 0, fmt.Errorf("token consumption should not be allowed with zero tokens and no refill")
				}
				return tokens, allowed, count, err
			},
		},
		{
			name: "zero refill rate - enough tokens",
			args: args{
				key:          "test_token_zero_refill_success",
				bucketSize:   10,
				refillRate:   0,
				refillAmount: 0,
				window:       time.Minute,
			},
			wantErr: false,
			verify: func() (float64, bool, int64, error) {
				// Set state with enough tokens
				require.NoError(t, backend.Set(ctx, "test_token_zero_refill_success", &BackendData{Tokens: 3}, 0))
				tokens, allowed, count, err := backend.ConsumeToken(ctx, "test_token_zero_refill_success", 10, 0, 0, time.Minute)
				if err != nil {
					return 0, false, 0, err
				}
				if !allowed {
					return 0, false, 0, fmt.Errorf("token consumption should be allowed with sufficient tokens")
				}
				if tokens != 2 {
					return 0, false, 0, fmt.Errorf("expected 2 tokens remaining, got %f", tokens)
				}
				return tokens, allowed, count, err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.verify != nil {
				_, _, _, err := tt.verify()
				if (err != nil) != tt.wantErr {
					t.Errorf("MemoryBackend.ConsumeToken() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			}
		})
	}
}

func TestMemoryBackend_Delete(t *testing.T) {
	ctx := t.Context()

	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Hour,
		MaxKeys:         1000,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
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
				_ = backend.Set(ctx, "test_delete", data, 0)
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
				t.Errorf("MemoryBackend.Delete() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify deletion
			got, _ := backend.Get(ctx, tt.args.key)
			if got != nil {
				t.Errorf("MemoryBackend.Delete() key still exists after deletion")
			}
		})
	}
}

func TestMemoryBackend_Close(t *testing.T) {
	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Hour,
		MaxKeys:         1000,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}

	if err := backend.Close(); err != nil {
		t.Errorf("MemoryBackend.Close() error = %v", err)
	}

	// Verify that data is cleared
	stats := backend.GetStats()
	if stats.KeyCount != 0 {
		t.Errorf("MemoryBackend.Close() expected KeyCount=0 after close, got %d", stats.KeyCount)
	}
}

func TestMemoryBackend_GetStats(t *testing.T) {
	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: 30 * time.Second,
		MaxKeys:         500,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}
	defer backend.Close()

	ctx := t.Context()

	// Add some test data
	_ = backend.Set(ctx, "key1", &BackendData{Count: 1}, 0)
	_ = backend.Set(ctx, "key2", &BackendData{Count: 2}, 0)

	stats := backend.GetStats()
	if stats.KeyCount != 2 {
		t.Errorf("MemoryBackend.GetStats() KeyCount = %d, want 2", stats.KeyCount)
	}
	if stats.MaxKeys != 500 {
		t.Errorf("MemoryBackend.GetStats() MaxKeys = %d, want 500", stats.MaxKeys)
	}
	if stats.CleanupInterval != 30*time.Second {
		t.Errorf("MemoryBackend.GetStats() CleanupInterval = %v, want 30s", stats.CleanupInterval)
	}
}

func TestMemoryBackend_cleanupLoop(t *testing.T) {
	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: 10 * time.Millisecond, // Very short interval for testing
		MaxKeys:         1000,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}

	ctx := t.Context()

	// Add some test data with old timestamps
	oldTime := time.Now().Add(-time.Hour)
	_ = backend.Set(ctx, "old_key1", &BackendData{Count: 1, LastRequest: oldTime}, 0)
	_ = backend.Set(ctx, "old_key2", &BackendData{Count: 2, LastRequest: oldTime}, 0)
	_ = backend.Set(ctx, "new_key", &BackendData{Count: 3, LastRequest: time.Now()}, 0)

	// Let the cleanup loop run a few times
	time.Sleep(50 * time.Millisecond)

	// Close to stop the cleanup loop
	backend.Close()

	// Check if old keys were cleaned up
	stats := backend.GetStats()
	if stats.KeyCount > 1 {
		// Should only have the new key left
		t.Errorf("MemoryBackend_cleanupLoop() expected cleanup of old keys, got %d keys", stats.KeyCount)
	}
}

func TestMemoryBackend_cleanup(t *testing.T) {
	// Use a 1 second cleanup interval for testing (so 2*cleanupInterval = 2 seconds)
	// This way old keys (5 seconds old) will be cleaned up
	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Second,
		MaxKeys:         1000,
	})
	if err != nil {
		t.Fatalf("Failed to create memory backend: %v", err)
	}
	defer backend.Close()

	ctx := t.Context()

	// Add some test data with different timestamps
	oldTime := time.Now().Add(-5 * time.Second) // Older than 2 * cleanupInterval (2 seconds)
	recentTime := time.Now().Add(-1 * time.Second)

	require.NoError(t, backend.Set(ctx, "old_key", &BackendData{Count: 1, LastRequest: oldTime}, 0))
	require.NoError(t, backend.Set(ctx, "recent_key", &BackendData{Count: 2, LastRequest: recentTime}, 0))
	require.NoError(t, backend.Set(ctx, "new_key", &BackendData{Count: 3, LastRequest: time.Now()}, 0))

	// Run cleanup manually
	backend.cleanup()

	// Check cleanup results - the old key should have been cleaned up
	stats := backend.GetStats()

	// The old key should have been cleaned up, leaving 2 keys
	if stats.KeyCount != 2 {
		t.Errorf("MemoryBackend_cleanup() expected 2 keys after cleanup, got %d", stats.KeyCount)
	}

	// Verify the old key was cleaned up
	if data, err := backend.Get(ctx, "old_key"); err == nil && data != nil {
		t.Errorf("MemoryBackend_cleanup() old_key should have been cleaned up, but got data: %+v", data)
	}

	// Verify the other keys still exist
	if data, err := backend.Get(ctx, "recent_key"); err != nil || data == nil {
		t.Errorf("MemoryBackend_cleanup() recent_key should not have been cleaned up")
	}
	if data, err := backend.Get(ctx, "new_key"); err != nil || data == nil {
		t.Errorf("MemoryBackend_cleanup() new_key should not have been cleaned up")
	}
}

// TestMemoryBackend_Integration tests the complete memory backend workflow
func TestMemoryBackend_Integration(t *testing.T) {
	ctx := t.Context()

	backend, err := NewMemoryBackend(BackendConfig{
		CleanupInterval: time.Hour,
		MaxKeys:         1000,
	})
	require.NoErrorf(t, err, "Failed to create memory backend: %v", err)
	defer backend.Close()

	// Test complete workflow
	key := "integration_test"

	// 1. Set initial data
	data := &BackendData{
		Count:       5,
		WindowStart: time.Now(),
		LastRequest: time.Now(),
	}
	err = backend.Set(ctx, key, data, time.Minute)
	require.NoErrorf(t, err, "Failed to set initial data: %v", err)

	// 2. Get and verify
	retrieved, err := backend.Get(ctx, key)
	require.NoErrorf(t, err, "Failed to get data: %v", err)
	require.Condition(t, func() bool {
		return retrieved != nil && retrieved.Count == 5
	}, "Retrieved data mismatch: got %+v", retrieved)

	// 3. Test increment (fixed window) - use a fresh key for increment test
	incrementKey := "increment_test"
	count, incremented, err := backend.Increment(ctx, incrementKey, time.Minute, 10)
	require.NoErrorf(t, err, "Failed to increment: %v", err)
	require.Conditionf(t, func() bool {
		return incremented && count == 1
	}, "Increment failed: count=%d, incremented=%v", count, incremented)

	// 4. Test token bucket
	tokenKey := "token_test"
	tokens, consumed, requests, err := backend.ConsumeToken(ctx, tokenKey, 10, time.Second, 1, time.Minute)
	require.NoErrorf(t, err, "Failed to consume token: %v", err)
	if !consumed || requests != 1 || tokens < 9 {
		t.Fatalf("Token consumption failed: tokens=%f, consumed=%v, requests=%d", tokens, consumed, requests)
	}

	// 5. Test stats
	stats := backend.GetStats()
	if stats.KeyCount < 2 { // Should have at least 'integration_test' and 'token_test'
		t.Fatalf("Expected at least 2 keys, got %d", stats.KeyCount)
	}

	// 6. Test cleanup
	backend.cleanup()

	// 7. Clean up
	err = backend.Delete(ctx, key)
	require.NoErrorf(t, err, "Failed to delete key: %v", err)
	err = backend.Delete(ctx, incrementKey)
	require.NoErrorf(t, err, "Failed to delete increment key: %v", err)
	err = backend.Delete(ctx, tokenKey)
	require.NoErrorf(t, err, "Failed to delete token key: %v", err)

	// 8. Verify deletion
	retrieved, err = backend.Get(ctx, key)
	require.NoErrorf(t, err, "Failed to get after deletion: %v", err)
	require.Nilf(t, retrieved, "Data still exists after deletion")
}
