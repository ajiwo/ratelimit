package backends

import (
	"testing"
	"time"
)

func TestNewBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  BackendConfig
		wantErr bool
		skip    bool
	}{
		{
			name: "memory backend",
			config: BackendConfig{
				Type: "memory",
			},
			wantErr: false,
		},
		{
			name: "redis backend",
			config: BackendConfig{
				Type:         "redis",
				RedisAddress: "localhost:6379",
			},
			wantErr: false,
			skip:    true, // Skip if Redis is not available
		},
		{
			name: "unsupported backend",
			config: BackendConfig{
				Type: "unsupported",
			},
			wantErr: true,
		},
		{
			name:    "empty backend type",
			config:  BackendConfig{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				// Try to create a Redis backend to check if Redis is available
				_, err := NewRedisBackend(BackendConfig{RedisAddress: "localhost:6379"})
				if err != nil {
					t.Skipf("Skipping test: Redis not available: %v", err)
				}
			}

			got, err := NewBackend(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Errorf("NewBackend() returned nil when no error expected")
			}
			if !tt.wantErr && got != nil {
				_ = got.Close()
			}
		})
	}
}

func TestValidateBackendConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  BackendConfig
		wantErr bool
	}{
		{
			name: "valid memory config",
			config: BackendConfig{
				Type:            "memory",
				MaxKeys:         1000,
				CleanupInterval: time.Minute,
			},
			wantErr: false,
		},
		{
			name: "memory config with negative max keys",
			config: BackendConfig{
				Type:    "memory",
				MaxKeys: -1,
			},
			wantErr: true,
		},
		{
			name: "memory config with negative cleanup interval",
			config: BackendConfig{
				Type:            "memory",
				CleanupInterval: -time.Minute,
			},
			wantErr: true,
		},
		{
			name: "valid redis config",
			config: BackendConfig{
				Type:          "redis",
				RedisAddress:  "localhost:6379",
				RedisPoolSize: 10,
			},
			wantErr: false,
		},
		{
			name: "redis config with empty address",
			config: BackendConfig{
				Type: "redis",
			},
			wantErr: true,
		},
		{
			name: "redis config with zero pool size",
			config: BackendConfig{
				Type:          "redis",
				RedisAddress:  "localhost:6379",
				RedisPoolSize: 0,
			},
			wantErr: true,
		},
		{
			name: "redis config with negative pool size",
			config: BackendConfig{
				Type:          "redis",
				RedisAddress:  "localhost:6379",
				RedisPoolSize: -1,
			},
			wantErr: true,
		},
		{
			name: "unsupported backend type",
			config: BackendConfig{
				Type: "unsupported",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBackendConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBackendConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetSupportedBackends(t *testing.T) {
	backends := GetSupportedBackends()

	if len(backends) != 2 {
		t.Errorf("GetSupportedBackends() returned %d backends, expected 2", len(backends))
	}

	hasMemory := false
	hasRedis := false

	for _, backend := range backends {
		if backend == "memory" {
			hasMemory = true
		}
		if backend == "redis" {
			hasRedis = true
		}
	}

	if !hasMemory {
		t.Errorf("GetSupportedBackends() missing 'memory' backend")
	}

	if !hasRedis {
		t.Errorf("GetSupportedBackends() missing 'redis' backend")
	}
}
