package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	state := GCRA{
		TAT: now,
	}

	encoded := encodeState(state)
	decoded, ok := decodeState(encoded)

	assert.True(t, ok)
	assert.Equal(t, state.TAT.UnixNano(), decoded.TAT.UnixNano())
}

func TestDecodeStateInvalid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid header",
			input: "v1|123456789",
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
			name:  "invalid TAT",
			input: "v2|abc",
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

	t.Run("valid TAT", func(t *testing.T) {
		data := "1234567890"
		tat, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, int64(1234567890), tat)
	})

	t.Run("valid zero TAT", func(t *testing.T) {
		data := "0"
		tat, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, int64(0), tat)
	})

	t.Run("valid negative TAT", func(t *testing.T) {
		data := "-1234567890"
		tat, ok := parseStateFields(data)

		assert.True(t, ok)
		assert.Equal(t, int64(-1234567890), tat)
	})

	t.Run("invalid TAT", func(t *testing.T) {
		data := "abc"
		_, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("empty TAT", func(t *testing.T) {
		data := ""
		_, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("TAT with whitespace", func(t *testing.T) {
		data := " 1234567890 "
		_, ok := parseStateFields(data)
		assert.False(t, ok)
	})

	t.Run("TAT with newline", func(t *testing.T) {
		data := "1234567890\n"
		_, ok := parseStateFields(data)
		assert.False(t, ok)
	})
}
