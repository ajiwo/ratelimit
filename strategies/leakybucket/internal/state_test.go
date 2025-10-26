package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	bucket := LeakyBucket{
		Requests: 123.45,
		LastLeak: now,
	}

	encoded := encodeState(bucket)
	decoded, ok := decodeState(encoded)

	assert.True(t, ok)
	assert.Equal(t, bucket.Requests, decoded.Requests)
	assert.Equal(t, bucket.LastLeak.UnixNano(), decoded.LastLeak.UnixNano())
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
			name:  "invalid requests",
			input: "v2|abc|123456789",
		},
		{
			name:  "invalid last leak",
			input: "v2|123.45|abc",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "only header",
			input: "v2|",
		},
		{
			name:  "missing second field",
			input: "v2|123.45|",
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

func TestParseStateFields(t *testing.T) {
	t.Parallel()

	t.Run("valid fields", func(t *testing.T) {
		data := "123.45|1234567890"
		requests, last, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, 123.45, requests)
		assert.Equal(t, int64(1234567890), last)
	})

	t.Run("valid integer requests", func(t *testing.T) {
		data := "42|1234567890"
		requests, last, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, 42.0, requests)
		assert.Equal(t, int64(1234567890), last)
	})

	t.Run("valid zero requests", func(t *testing.T) {
		data := "0|1234567890"
		requests, last, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, 0.0, requests)
		assert.Equal(t, int64(1234567890), last)
	})

	t.Run("missing separator", func(t *testing.T) {
		data := "123.451234567890"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("invalid requests", func(t *testing.T) {
		data := "abc|1234567890"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("invalid last leak", func(t *testing.T) {
		data := "123.45|abc"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("empty fields", func(t *testing.T) {
		data := "|"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("negative requests", func(t *testing.T) {
		data := "-5.5|1234567890"
		requests, last, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, -5.5, requests)
		assert.Equal(t, int64(1234567890), last)
	})
}
