package backends

import (
	"fmt"
)

// NewStorage creates a new storage instance based on the configuration
func NewStorage(storageType string, config BackendConfig) (Storage, error) {
	switch storageType {
	case "memory":
		return NewMemoryStorage(config)
	case "redis":
		return NewRedisStorage(config)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

// NewBackend creates a new backend instance based on the configuration (for backward compatibility)
func NewBackend(config BackendConfig) (Backend, error) {
	switch config.Type {
	case "memory":
		return NewMemoryBackend(config)
	case "redis":
		return NewRedisBackend(config)
	default:
		return nil, fmt.Errorf("unsupported backend type: %s", config.Type)
	}
}

// ValidateBackendConfig validates the backend configuration
func ValidateBackendConfig(config BackendConfig) error {
	switch config.Type {
	case "memory":
		if config.MaxKeys < 0 {
			return fmt.Errorf("memory backend max_keys must be non-negative")
		}
		if config.CleanupInterval < 0 {
			return fmt.Errorf("memory backend cleanup_interval must be non-negative")
		}
		return nil
	case "redis":
		if config.RedisAddress == "" {
			return fmt.Errorf("redis backend requires address")
		}
		if config.RedisPoolSize <= 0 {
			return fmt.Errorf("redis backend pool_size must be greater than 0")
		}
		return nil
	default:
		return fmt.Errorf("unsupported backend type: %s", config.Type)
	}
}

// GetSupportedBackends returns a list of supported backend types
func GetSupportedBackends() []string {
	return []string{"memory", "redis"}
}
