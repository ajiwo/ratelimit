package strategies

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

type regMockBackend struct{}

func (d *regMockBackend) Get(_ context.Context, _ string) (string, error) { return "", nil }
func (d *regMockBackend) Set(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}
func (d *regMockBackend) CheckAndSet(_ context.Context, _ string, _, _ string, _ time.Duration) (bool, error) {
	return true, nil
}
func (d *regMockBackend) Delete(_ context.Context, _ string) error { return nil }
func (d *regMockBackend) Close() error                             { return nil }

// Ensure interface compliance
var _ backends.Backend = (*regMockBackend)(nil)

func TestStrategiesRegistry_RegisterAndCreate(t *testing.T) {
	id := StrategyID(200)
	created := false
	Register(id, func(_ backends.Backend) Strategy {
		created = true
		return nil
	})

	if _, err := Create(StrategyID(201), &regMockBackend{}); !errors.Is(err, ErrStrategyNotFound) {
		t.Fatalf("expected ErrStrategyNotFound, got %v", err)
	}

	_, err := Create(id, &regMockBackend{})
	if err != nil || !created {
		t.Fatal("error creating strategy")
	}
}
