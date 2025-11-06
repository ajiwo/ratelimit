package strategies

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalcExpiration(t *testing.T) {
	// capacity 10, rate 5 -> (10/5)*2 = 4 seconds
	d := CalcExpiration(10, 5)
	assert.Equal(t, 4*time.Second, d)

	// Very small -> min 1 second
	d = CalcExpiration(1, 1000)
	assert.Equal(t, time.Second, d)

	// Non-integer seconds should truncate after multiplication logic: (3/2)*2 = 3 -> 3s
	d = CalcExpiration(3, 2)
	assert.Equal(t, 3*time.Second, d)
}

func TestNextDelay(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		feedback    time.Duration
		baseDelay   time.Duration // expected delay without jitter
		minExpected time.Duration // minimum expected with jitter (half of base)
		maxExpected time.Duration // maximum expected with jitter (base delay)
	}{
		{
			name:        "zero attempt with normal feedback",
			attempt:     0,
			feedback:    100 * time.Millisecond,
			baseDelay:   100 * time.Millisecond, // (100ms * 1) << 0
			minExpected: 50 * time.Millisecond,  // half of base
			maxExpected: 100 * time.Millisecond, // base delay
		},
		{
			name:        "first attempt with normal feedback",
			attempt:     1,
			feedback:    100 * time.Millisecond,
			baseDelay:   400 * time.Millisecond, // (100ms * 2) << 1
			minExpected: 200 * time.Millisecond, // half of base
			maxExpected: 400 * time.Millisecond, // base delay
		},
		{
			name:        "feedback below minimum (30ns) gets clamped",
			attempt:     1,
			feedback:    1 * time.Nanosecond,
			baseDelay:   120 * time.Nanosecond, // (1ns -> 30ns * 2) << 1
			minExpected: 60 * time.Nanosecond,  // half of base
			maxExpected: 120 * time.Nanosecond, // base delay
		},
		{
			name:        "feedback above maximum (30s) gets clamped",
			attempt:     1,
			feedback:    60 * time.Second,
			baseDelay:   120 * time.Second, // (60s -> 30s * 2) << 1
			minExpected: 60 * time.Second,  // half of base
			maxExpected: 120 * time.Second, // base delay
		},
		{
			name:        "exponential backoff with shift",
			attempt:     2,
			feedback:    100 * time.Millisecond,
			baseDelay:   1200 * time.Millisecond, // (100ms * 3) << 2
			minExpected: 600 * time.Millisecond,  // half of base
			maxExpected: 1200 * time.Millisecond, // base delay
		},
		{
			name:        "higher attempt with shift wrapping",
			attempt:     16,
			feedback:    100 * time.Millisecond,
			baseDelay:   1700 * time.Millisecond, // (100ms * 17) << 0 (shift wraps: 16 % 16 = 0)
			minExpected: 850 * time.Millisecond,  // half of base
			maxExpected: 1700 * time.Millisecond, // base delay
		},
		{
			name:        "large shift value",
			attempt:     4,
			feedback:    50 * time.Millisecond,
			baseDelay:   4000 * time.Millisecond, // (50ms * 5) << 3
			minExpected: 2000 * time.Millisecond, // half of base
			maxExpected: 4000 * time.Millisecond, // base delay
		},
		{
			name:        "boundary feedback - exactly 10ms",
			attempt:     3,
			feedback:    10 * time.Millisecond,
			baseDelay:   320 * time.Millisecond, // (10ms * 4) << 3
			minExpected: 160 * time.Millisecond, // half of base
			maxExpected: 320 * time.Millisecond, // base delay
		},
		{
			name:        "boundary feedback - exactly 10s",
			attempt:     2,
			feedback:    10 * time.Second,
			baseDelay:   120 * time.Second, // (10s * 3) << 2
			minExpected: 60 * time.Second,  // half of base
			maxExpected: 120 * time.Second, // base delay
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextDelay(tt.attempt, tt.feedback)
			// With jitter, the result should be between half and the full base delay
			assert.True(t, got >= tt.minExpected, "delay %v should be >= min expected %v", got, tt.minExpected)
			assert.True(t, got < tt.maxExpected, "delay %v should be < max expected %v", got, tt.maxExpected)
		})
	}
}

func TestNextDelay_SawtoothPattern(t *testing.T) {
	// Test that the delay pattern exhibits sawtooth behavior
	feedback := 50 * time.Millisecond

	// run multiple iterations to establish a pattern
	// then verify the overall trend by checking average behavior
	var sumDelays [20]time.Duration
	iterations := 100

	for range iterations {
		for attempt := range 20 {
			sumDelays[attempt] += NextDelay(attempt, feedback)
		}
	}

	var avgDelays [20]time.Duration
	for attempt := range 20 {
		avgDelays[attempt] = sumDelays[attempt] / time.Duration(iterations)
	}

	// First few attempts should show exponential growth (even with jitter averaging)
	assert.True(t, avgDelays[1] > avgDelays[0], "attempt 1 should be > attempt 0")
	assert.True(t, avgDelays[2] > avgDelays[1], "attempt 2 should be > attempt 1")
	assert.True(t, avgDelays[3] > avgDelays[2], "attempt 3 should be > attempt 2")

	// At attempt 16, shift should wrap around, causing a "reset" in the exponential pattern
	// avgDelays[16] should be significantly less than avgDelays[15] due to shift reset
	assert.True(t, avgDelays[16] < avgDelays[15], "attempt 16 should be < attempt 15 due to shift reset")
	// but still larger than avgDelays[0] due to attempt multiplier
	assert.True(t, avgDelays[16] > avgDelays[0], "attempt 16 should be > attempt 0 due to attempt multiplier")

	// The approximate relationship should hold (with some tolerance for jitter)
	expectedRatio := 17.0
	actualRatio := float64(avgDelays[16]) / float64(avgDelays[0])
	assert.InDelta(t, expectedRatio, actualRatio, 2.0, "attempt 16 should be approximately 17x attempt 0")
}
