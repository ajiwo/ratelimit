package internal

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReset(t *testing.T) {
	ctx := t.Context()
	key := "test-reset-key"

	t.Run("successful reset", func(t *testing.T) {
		storage := new(mockBackend)
		config := new(mockConfig)

		config.On("GetKey").Return(key)
		storage.On("Delete", ctx, key).Return(nil)

		err := Reset(ctx, config, storage)
		assert.NoError(t, err)

		// Verify Delete was called with expected parameters
		storage.AssertExpectations(t)
		config.AssertExpectations(t)
	})

	t.Run("delete failed", func(t *testing.T) {
		storage := new(mockBackend)
		config := new(mockConfig)

		config.On("GetKey").Return(key)
		storage.On("Delete", ctx, key).Return(errors.New("delete failed"))

		err := Reset(ctx, config, storage)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delete failed")

		storage.AssertExpectations(t)
		config.AssertExpectations(t)
	})
}
