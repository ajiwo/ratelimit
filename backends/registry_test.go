package backends

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRegister(t *testing.T) {
	// Clear any existing backends for clean test
	registeredBackends = make(map[string]BackendFactory)

	// Test registering a new backend
	factory := func(config any) (Backend, error) {
		return &mockBackend{}, nil
	}

	Register("test", factory)

	// Verify backend was registered
	assert.Contains(t, registeredBackends, "test")
	assert.NotNil(t, registeredBackends["test"])

	// Test registering duplicate (should overwrite)
	newFactory := func(config any) (Backend, error) {
		return &mockBackend{name: "new"}, nil
	}
	Register("test", newFactory)

	assert.NotNil(t, registeredBackends["test"])
}

func TestCreate(t *testing.T) {
	// Clear any existing backends for clean test
	registeredBackends = make(map[string]BackendFactory)

	// Test creating non-existent backend
	backend, err := Create("nonexistent", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrBackendNotFound, err)
	assert.Nil(t, backend)

	// Test creating existing backend without config
	expectedBackend := &mockBackend{name: "test"}
	factory := func(config any) (Backend, error) {
		return expectedBackend, nil
	}
	Register("test", factory)

	backend, err = Create("test", nil)
	assert.NoError(t, err)
	assert.NotNil(t, backend)

	// Test creating backend with config
	backend, err = Create("test", "some config")
	assert.NoError(t, err)
	assert.NotNil(t, backend)

	// Test creating backend that returns error
	errorFactory := func(config any) (Backend, error) {
		return nil, errors.New("test error")
	}
	Register("error", errorFactory)

	backend, err = Create("error", nil)
	assert.Error(t, err)
	assert.Equal(t, "test error", err.Error())
	assert.Nil(t, backend)
}

func TestCreateBackends(t *testing.T) {
	// Clear any existing backends for clean test
	registeredBackends = make(map[string]BackendFactory)

	// Test memory backend (no config needed)
	memoryBackend := &mockBackend{name: "memory"}
	Register("memory", func(config any) (Backend, error) {
		return memoryBackend, nil
	})

	backend, err := Create("memory", nil)
	assert.NoError(t, err)
	assert.NotNil(t, backend)

	// Test Redis backend with valid config
	redisBackend := &mockBackend{name: "redis"}
	Register("redis", func(config any) (Backend, error) {
		redisConfig, ok := config.(RedisConfig)
		if !ok || redisConfig.Addr == "" {
			return nil, ErrInvalidConfig
		}
		return redisBackend, nil
	})

	redisConfig := RedisConfig{
		Addr:     "localhost:6379",
		Password: "secret",
		DB:       1,
		PoolSize: 10,
	}
	backend, err = Create("redis", redisConfig)
	assert.NoError(t, err)
	assert.NotNil(t, backend)

	// Test Redis backend with invalid config (missing addr)
	invalidRedisConfig := RedisConfig{
		Password: "secret",
		DB:       1,
		PoolSize: 10,
	}
	backend, err = Create("redis", invalidRedisConfig)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidConfig, err)
	assert.Nil(t, backend)

	// Test PostgreSQL backend with valid config
	postgresBackend := &mockBackend{name: "postgres"}
	Register("postgres", func(config any) (Backend, error) {
		pgConfig, ok := config.(PostgresConfig)
		if !ok || pgConfig.ConnString == "" {
			return nil, ErrInvalidConfig
		}
		return postgresBackend, nil
	})

	postgresConfig := PostgresConfig{
		ConnString: "postgres://user:pass@localhost/db",
		MaxConns:   10,
		MinConns:   2,
	}
	backend, err = Create("postgres", postgresConfig)
	assert.NoError(t, err)
	assert.NotNil(t, backend)

	// Test PostgreSQL backend with invalid config (missing DSN)
	invalidPostgresConfig := PostgresConfig{}
	backend, err = Create("postgres", invalidPostgresConfig)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidConfig, err)
	assert.Nil(t, backend)

	// Test non-existent backend
	backend, err = Create("nonexistent", nil)
	assert.Error(t, err)
	assert.Equal(t, ErrBackendNotFound, err)
	assert.Nil(t, backend)
}

// mockBackend is a simple implementation for testing
type mockBackend struct {
	name string
}

func (m *mockBackend) Get(ctx context.Context, key string) (string, error) {
	return "mock-value", nil
}

func (m *mockBackend) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return nil
}

func (m *mockBackend) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockBackend) Close() error {
	return nil
}
