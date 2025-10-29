package backends

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBackend struct{}

func (d *mockBackend) Get(_ context.Context, _ string) (string, error)                  { return "", nil }
func (d *mockBackend) Set(_ context.Context, _ string, _ string, _ time.Duration) error { return nil }
func (d *mockBackend) CheckAndSet(_ context.Context, _ string, _, _ string, _ time.Duration) (bool, error) {
	return true, nil
}
func (d *mockBackend) Delete(_ context.Context, _ string) error { return nil }
func (d *mockBackend) Close() error                             { return nil }

// Ensure interface compliance
var _ Backend = (*mockBackend)(nil)

func TestBackendsRegistry_RegisterAndCreate(t *testing.T) {
	// Register a backend factory
	name := "dummy"
	Register(name, func(_ any) (Backend, error) { return &mockBackend{}, nil })

	// Create should return an instance
	b, err := Create(name, nil)
	require.NoError(t, err)
	assert.NotNil(t, b)

	// Unknown backend should return ErrBackendNotFound
	b, err = Create("missing", nil)
	require.IsType(t, err, ErrBackendNotFound)
	assert.Nil(t, b)
}
