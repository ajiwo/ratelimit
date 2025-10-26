package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	window := FixedWindow{
		Count: 42,
		Start: now,
	}

	encoded := encodeState(window)
	decoded, ok := decodeState(encoded)

	assert.True(t, ok)
	assert.Equal(t, window.Count, decoded.Count)
	assert.Equal(t, window.Start.UnixNano(), decoded.Start.UnixNano())
}

func TestDecodeStateInvalid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid header",
			input: "v1|42|123456789",
		},
		{
			name:  "missing parts",
			input: "v2|42",
		},
		{
			name:  "invalid count",
			input: "v2|abc|123456789",
		},
		{
			name:  "invalid start time",
			input: "v2|42|abc",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "only header",
			input: "v2|",
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
		data := "42|1234567890"
		count, startNS, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, 42, count)
		assert.Equal(t, int64(1234567890), startNS)
	})

	t.Run("missing separator", func(t *testing.T) {
		data := "421234567890"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("invalid count", func(t *testing.T) {
		data := "abc|1234567890"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("invalid start time", func(t *testing.T) {
		data := "42|abc"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("empty fields", func(t *testing.T) {
		data := "|"
		_, _, ok := parseStateFields(data)
		assert.False(t, ok)
	})
}

func TestBuildQuotaKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		baseKey   string
		quotaName string
		expected  string
	}{
		{
			name:      "simple keys",
			baseKey:   "api",
			quotaName: "requests",
			expected:  "api:requests",
		},
		{
			name:      "complex keys",
			baseKey:   "user:123",
			quotaName: "writes",
			expected:  "user:123:writes",
		},
		{
			name:      "empty quota name",
			baseKey:   "api",
			quotaName: "",
			expected:  "api:",
		},
		{
			name:      "empty base key",
			baseKey:   "",
			quotaName: "requests",
			expected:  ":requests",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := buildQuotaKey(tc.baseKey, tc.quotaName)
			assert.Equal(t, tc.expected, result)
		})
	}
}
