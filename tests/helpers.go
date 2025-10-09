package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/backends/memory"
	"github.com/ajiwo/ratelimit/backends/postgres"
	"github.com/ajiwo/ratelimit/backends/redis"
)

// UseBackend creates a backend instance for testing, skipping the test if the backend is not available
func UseBackend(t *testing.T, name string) backends.Backend {
	t.Helper()
	var backend backends.Backend
	var err error

	postgresConn := os.Getenv("TEST_POSTGRES_DSN")
	if postgresConn == "" {
		postgresConn = "postgres://postgres:postgres@localhost:5432/ratelimit_test?sslmode=disable"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")

	switch name {
	case "memory":
		backend = memory.New()
	case "postgres":
		backend, err = postgres.New(postgres.PostgresConfig{
			ConnString: postgresConn,
		})
	case "redis":
		backend, err = redis.New(redis.RedisConfig{
			Addr:     redisAddr,
			Password: redisPassword,
		})
	default:
		err = fmt.Errorf("unknown backend %s", name)
	}

	if err != nil {
		t.Skipf("Backend %s not available, skipping tests: %v", name, err)
	}

	return backend
}

// AvailableBackends returns a list of backends that are available for testing
func AvailableBackends(t *testing.T) []string {
	t.Helper()
	var available []string

	// Test memory backend (always available)
	if memory.New() != nil {
		available = append(available, "memory")
	}

	// Test postgres backend
	if _, err := postgres.New(postgres.PostgresConfig{
		ConnString: os.Getenv("TEST_POSTGRES_DSN"),
	}); err == nil {
		available = append(available, "postgres")
	}

	// Test redis backend
	if _, err := redis.New(redis.RedisConfig{
		Addr: os.Getenv("REDIS_ADDR"),
	}); err == nil {
		available = append(available, "redis")
	}

	return available
}
