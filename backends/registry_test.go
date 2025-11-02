package backends

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBackend struct {
	data map[string]string
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		data: make(map[string]string),
	}
}

func (d *mockBackend) Get(_ context.Context, key string) (string, error) {
	if val, exists := d.data[key]; exists {
		return val, nil
	}
	return "", nil
}

func (d *mockBackend) Set(_ context.Context, key string, value string, _ time.Duration) error {
	d.data[key] = value
	return nil
}

func (d *mockBackend) CheckAndSet(_ context.Context, key string, oldValue, newValue string, _ time.Duration) (bool, error) {
	currentValue, exists := d.data[key]

	if oldValue == "" {
		// Only set if key doesn't exist
		if exists {
			return false, nil
		}
		d.data[key] = newValue
		return true, nil
	}

	// Check if current value matches oldValue
	if !exists || currentValue != oldValue {
		return false, nil
	}

	// Value matches, update it
	d.data[key] = newValue
	return true, nil
}

func (d *mockBackend) Delete(_ context.Context, key string) error {
	delete(d.data, key)
	return nil
}

func (d *mockBackend) Close() error {
	d.data = nil
	return nil
}

// Ensure interface compliance
var _ Backend = (*mockBackend)(nil)

func TestBackendsRegistry_RegisterAndCreate(t *testing.T) {
	// Register a backend factory
	name := "dummy"
	Register(name, func(_ any) (Backend, error) { return newMockBackend(), nil })

	// Create should return an instance
	b, err := Create(name, nil)
	require.NoError(t, err)
	assert.NotNil(t, b)

	// Unknown backend should return ErrBackendNotFound
	b, err = Create("missing", nil)
	require.IsType(t, err, ErrBackendNotFound)
	assert.Nil(t, b)
}

func TestMockBackend_CheckAndSet(t *testing.T) {
	mock := newMockBackend()
	ctx := context.Background()

	// Test empty oldValue (set if not exists)
	success, err := mock.CheckAndSet(ctx, "key1", "", "value1", 0)
	require.NoError(t, err)
	assert.True(t, success, "Should succeed when key doesn't exist")

	// Test that subsequent empty oldValue fails
	success, err = mock.CheckAndSet(ctx, "key1", "", "value2", 0)
	require.NoError(t, err)
	assert.False(t, success, "Should fail when key already exists")

	// Verify value is unchanged
	val, err := mock.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", val, "Value should remain unchanged after failed CheckAndSet")

	// Test correct oldValue matching
	success, err = mock.CheckAndSet(ctx, "key1", "value1", "value2", 0)
	require.NoError(t, err)
	assert.True(t, success, "Should succeed when oldValue matches current value")

	// Verify value was updated
	val, err = mock.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value2", val, "Value should be updated on successful CheckAndSet")

	// Test incorrect oldValue matching
	success, err = mock.CheckAndSet(ctx, "key1", "wrong", "value3", 0)
	require.NoError(t, err)
	assert.False(t, success, "Should fail when oldValue doesn't match current value")

	// Verify value is unchanged after failed match attempt
	val, err = mock.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value2", val, "Value should remain unchanged after failed match attempt")

	// Test CheckAndSet on non-existent key
	success, err = mock.CheckAndSet(ctx, "nonexistent", "somevalue", "newvalue", 0)
	require.NoError(t, err)
	assert.False(t, success, "Should fail when key doesn't exist and oldValue is not empty")
}
