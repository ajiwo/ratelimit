package backends

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrUnhealthy(t *testing.T) {
	err := ErrUnhealthy
	assert.Equal(t, "backend unhealthy", err.Error())
}

func TestHealthError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *HealthError
		want string
	}{
		{
			name: "nil health error",
			err:  nil,
			want: "backend unhealthy",
		},
		{
			name: "health error with op and cause",
			err:  &HealthError{Op: "redis:Ping", Cause: errors.New("connection refused")},
			want: "backend unhealthy: redis:Ping: connection refused",
		},
		{
			name: "health error with only cause",
			err:  &HealthError{Cause: errors.New("connection refused")},
			want: "backend unhealthy: connection refused",
		},
		{
			name: "health error with empty op and cause",
			err:  &HealthError{Op: "", Cause: errors.New("connection refused")},
			want: "backend unhealthy: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				// Test nil case separately
				var he *HealthError
				assert.Equal(t, tt.want, he.Error())
			} else {
				assert.Equal(t, tt.want, tt.err.Error())
			}
		})
	}
}

func TestHealthError_Unwrap(t *testing.T) {
	cause := errors.New("connection refused")
	err := &HealthError{Op: "redis:Ping", Cause: cause}
	assert.Equal(t, cause, err.Unwrap())
}

func TestNewHealthError(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewHealthError("redis:Ping", cause)

	// Check that it's a health error
	assert.True(t, IsHealthError(err))

	// Check that we can unwrap to the sentinel
	assert.True(t, errors.Is(err, ErrUnhealthy))

	// Check that we can unwrap to the HealthError
	var he *HealthError
	assert.True(t, errors.As(err, &he))
	assert.Equal(t, "redis:Ping", he.Op)
	assert.Equal(t, cause, he.Cause)

	// Check that we can unwrap to the cause
	assert.Equal(t, cause, errors.Unwrap(err))
}

func TestIsHealthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "sentinel error",
			err:  ErrUnhealthy,
			want: true,
		},
		{
			name: "wrapped health error",
			err:  NewHealthError("redis:Ping", errors.New("connection refused")),
			want: true,
		},
		{
			name: "regular error",
			err:  errors.New("regular error"),
			want: false,
		},
		{
			name: "wrapped regular error",
			err:  fmt.Errorf("wrapped: %w", errors.New("regular error")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsHealthError(tt.err))
		})
	}
}

func TestHealthError_ErrorChaining(t *testing.T) {
	cause := errors.New("connection refused")
	err := NewHealthError("redis:Ping", cause)

	// Test error message
	expected := "backend unhealthy: redis:Ping: connection refused"
	assert.Equal(t, expected, err.Error())

	// Test that we can check for the sentinel
	assert.True(t, errors.Is(err, ErrUnhealthy))

	// Test that we can extract the HealthError
	var he *HealthError
	require.True(t, errors.As(err, &he))
	assert.Equal(t, "redis:Ping", he.Op)
	assert.Equal(t, cause, he.Cause)

	// Test that we can get to the underlying cause
	assert.Equal(t, cause, errors.Unwrap(he))
}

func TestMaybeConnError(t *testing.T) {
	tests := []struct {
		name         string
		op           string
		err          error
		patterns     []string
		expectHealth bool
		want         error
	}{
		{
			name:         "nil error",
			op:           "redis:Get",
			err:          nil,
			patterns:     []string{"connection refused"},
			expectHealth: false,
			want:         nil,
		},
		{
			name:         "matching pattern",
			op:           "redis:Get",
			err:          errors.New("Connection Refused"),
			patterns:     []string{"connection refused"},
			expectHealth: true,
		},
		{
			name:         "non-matching pattern",
			op:           "redis:Get",
			err:          errors.New("some other error"),
			patterns:     []string{"connection refused"},
			expectHealth: false,
			want:         errors.New("some other error"),
		},
		{
			name:         "nil patterns",
			op:           "redis:Get",
			err:          errors.New("connection refused"),
			patterns:     nil,
			expectHealth: false,
			want:         errors.New("connection refused"),
		},
		{
			name:         "empty patterns",
			op:           "redis:Get",
			err:          errors.New("connection refused"),
			patterns:     []string{},
			expectHealth: false,
			want:         errors.New("connection refused"),
		},
		{
			name:         "multiple patterns match",
			op:           "redis:Get",
			err:          errors.New("network timeout"),
			patterns:     []string{"connection refused", "timeout"},
			expectHealth: true,
		},
		{
			name:         "context deadline exceeded",
			op:           "redis:Get",
			err:          context.DeadlineExceeded,
			patterns:     nil,
			expectHealth: true,
		},
		{
			name:         "context canceled",
			op:           "redis:Get",
			err:          context.Canceled,
			patterns:     nil,
			expectHealth: true,
		},
		{
			name:         "no pattern match and not context error",
			op:           "redis:Get",
			err:          errors.New("invalid syntax"),
			patterns:     []string{"connection refused", "timeout"},
			expectHealth: false,
			want:         errors.New("invalid syntax"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaybeConnError(tt.op, tt.err, tt.patterns)

			if tt.expectHealth {
				// Check that it's a health error
				assert.True(t, IsHealthError(got))

				// Check that we can unwrap to the sentinel
				assert.True(t, errors.Is(got, ErrUnhealthy))

				// Check that we can unwrap to the HealthError
				var he *HealthError
				assert.True(t, errors.As(got, &he))
				assert.Equal(t, tt.op, he.Op)
				assert.Equal(t, tt.err, he.Cause)
			} else {
				// Should return the original error or nil
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
