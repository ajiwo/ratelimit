package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	quotaStates := map[string]FixedWindow{
		"default": {
			Count: 42,
			Start: now,
		},
		"burst": {
			Count: 5,
			Start: now.Add(-10 * time.Second),
		},
	}

	// Test encoding
	encoded := encodeState(quotaStates)
	assert.NotEmpty(t, encoded)
	assert.True(t, len(encoded) > 3) // Should have at least "v2|"

	// Test decoding
	decoded, ok := decodeState(encoded)
	assert.True(t, ok)
	assert.Len(t, decoded, 2)

	// Verify both expected keys exist
	assert.Contains(t, decoded, "burst")
	assert.Contains(t, decoded, "default")

	// Check values
	assert.Equal(t, 5, decoded["burst"].Count)
	assert.Equal(t, now.Add(-10*time.Second).UnixNano(), decoded["burst"].Start.UnixNano())
	assert.Equal(t, 42, decoded["default"].Count)
	assert.Equal(t, now.UnixNano(), decoded["default"].Start.UnixNano())
}

func TestDecodeStateInvalid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "invalid header",
			input: "x2|2|quota1|1|123|quota2|2|456",
		},
		{
			name:  "missing parts",
			input: "v2|2|quota1|1",
		},
		{
			name:  "invalid quota count",
			input: "v2|abc|quota1|1|123",
		},
		{
			name:  "too many quotas",
			input: "v2|9|quota1|1|123|quota2|2|456|quota3|3|789|quota4|4|111|quota5|5|222|quota6|6|333|quota7|7|444|quota8|8|555|quota9|9|666",
		},
		{
			name:  "invalid count",
			input: "v2|1|quota1|abc|123",
		},
		{
			name:  "invalid start time",
			input: "v2|1|quota1|1|abc",
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

func TestComputeMaxResetTTL(t *testing.T) {
	t.Parallel()

	now := time.Now()
	quotaStates := map[string]FixedWindow{
		"default": {
			Count: 42,
			Start: now,
		},
		"burst": {
			Count: 5,
			Start: now.Add(-30 * time.Second),
		},
	}
	quotas := map[string]Quota{
		"default": {
			Limit:  100,
			Window: time.Minute,
		},
		"burst": {
			Limit:  10,
			Window: 30 * time.Second,
		},
	}

	ttl := computeMaxResetTTL(quotaStates, quotas, now)
	assert.True(t, ttl >= 30*time.Second) // Default window hasn't expired
	assert.True(t, ttl <= 60*time.Second) // Shouldn't be more than the max window
}
