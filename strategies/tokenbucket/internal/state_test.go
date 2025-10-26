package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	bucket := TokenBucket{
		Tokens:     123.45,
		LastRefill: now,
	}

	encoded := encodeState(bucket)
	decoded, ok := decodeState(encoded)

	assert.True(t, ok)
	assert.Equal(t, bucket.Tokens, decoded.Tokens)
	assert.Equal(t, bucket.LastRefill.UnixNano(), decoded.LastRefill.UnixNano())
}

func TestDecodeStateInvalid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid header",
			input: "v1|123.45|123456789",
		},
		{
			name:  "missing parts",
			input: "v2|123.45",
		},
		{
			name:  "invalid tokens",
			input: "v2|abc|123456789",
		},
		{
			name:  "invalid last refill",
			input: "v2|123.45|abc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, ok := decodeState(tc.input)
			assert.False(t, ok)
		})
	}
}
