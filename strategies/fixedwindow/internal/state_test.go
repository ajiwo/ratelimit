package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	quotaStates := []FixedWindow{
		{
			Name:  "default",
			Count: 42,
			Start: now,
		},
		{
			Name:  "burst",
			Count: 5,
			Start: now.Add(-10 * time.Second),
		},
	}

	// Test encoding
	encoded := encodeState(quotaStates)
	assert.NotEmpty(t, encoded)
	assert.True(t, len(encoded) > 3) // Should have at least "23|"

	// Test decoding
	decoded, ok := decodeState(encoded)
	assert.True(t, ok)
	assert.Len(t, decoded, 2)

	// Create a map for easier verification
	decodedMap := make(map[string]FixedWindow, len(decoded))
	for _, state := range decoded {
		decodedMap[state.Name] = state
	}

	// Verify both expected keys exist
	assert.Contains(t, decodedMap, "burst")
	assert.Contains(t, decodedMap, "default")

	// Check values
	assert.Equal(t, 5, decodedMap["burst"].Count)
	assert.Equal(t, now.Add(-10*time.Second).UnixNano(), decodedMap["burst"].Start.UnixNano())
	assert.Equal(t, 42, decodedMap["default"].Count)
	assert.Equal(t, now.UnixNano(), decodedMap["default"].Start.UnixNano())
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
			input: "23|2|quota1|1",
		},
		{
			name:  "invalid quota count",
			input: "23|abc|quota1|1|123",
		},
		{
			name:  "too many quotas",
			input: "23|9|quota1|1|123|quota2|2|456|quota3|3|789|quota4|4|111|quota5|5|222|quota6|6|333|quota7|7|444|quota8|8|555|quota9|9|666",
		},
		{
			name:  "invalid count",
			input: "23|1|quota1|abc|123",
		},
		{
			name:  "invalid start time",
			input: "23|1|quota1|1|abc",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "only header",
			input: "23|",
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
	quotaStates := []FixedWindow{
		{
			Name:  "default",
			Count: 42,
			Start: now,
		},
		{
			Name:  "burst",
			Count: 5,
			Start: now.Add(-30 * time.Second),
		},
	}
	quotas := []Quota{
		{
			Name:   "default",
			Limit:  100,
			Window: time.Minute,
		},
		{
			Name:   "burst",
			Limit:  10,
			Window: 30 * time.Second,
		},
	}

	ttl := computeMaxResetTTL(quotaStates, quotas, now)
	assert.True(t, ttl >= 30*time.Second)  // Default window hasn't expired
	assert.True(t, ttl <= 300*time.Second) // Shouldn't be more than the max window
}
