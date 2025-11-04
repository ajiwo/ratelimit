package strategies

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckV2Header(t *testing.T) {
	cases := []struct {
		s  string
		ok bool
	}{
		{"v2|payload", true},
		{"v2|", true},
		{"v3|", false},
		{"v2-", false},
		{"", false},
		{"v", false},
		{"v2", false},
	}
	for _, tc := range cases {
		if got := CheckV2Header(tc.s); got != tc.ok {
			t.Fatalf("CheckV2Header(%q)=%v want %v", tc.s, got, tc.ok)
		}
	}
}

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
		name     string
		attempt  int
		feedback time.Duration
		want     time.Duration
	}{
		{
			name:     "zero attempt with normal feedback",
			attempt:  0,
			feedback: 100 * time.Millisecond,
			want:     100 * time.Millisecond, // (100ms * 1) << 0
		},
		{
			name:     "first attempt with normal feedback",
			attempt:  1,
			feedback: 100 * time.Millisecond,
			want:     400 * time.Millisecond, // (100ms * 2) << 1
		},
		{
			name:     "negative attempt treated as zero",
			attempt:  -5,
			feedback: 100 * time.Millisecond,
			want:     100 * time.Millisecond, // (-5 -> 0) (100ms * 1) << 0
		},
		{
			name:     "feedback below minimum (10ms) gets clamped",
			attempt:  1,
			feedback: 1 * time.Millisecond,
			want:     40 * time.Millisecond, // (1ms -> 10ms * 2) << 1
		},
		{
			name:     "feedback above maximum (10s) gets clamped",
			attempt:  1,
			feedback: 30 * time.Second,
			want:     40 * time.Second, // (30s -> 10s * 2) << 1
		},
		{
			name:     "exponential backoff with shift",
			attempt:  2,
			feedback: 100 * time.Millisecond,
			want:     1200 * time.Millisecond, // (100ms * 3) << 2
		},
		{
			name:     "higher attempt with shift wrapping",
			attempt:  16,
			feedback: 100 * time.Millisecond,
			want:     1700 * time.Millisecond, // (100ms * 17) << 0 (shift wraps: 16 % 16 = 0)
		},
		{
			name:     "large shift value",
			attempt:  4,
			feedback: 50 * time.Millisecond,
			want:     4000 * time.Millisecond, // (50ms * 5) << 3
		},
		{
			name:     "boundary feedback - exactly 10ms",
			attempt:  3,
			feedback: 10 * time.Millisecond,
			want:     320 * time.Millisecond, // (10ms * 4) << 3
		},
		{
			name:     "boundary feedback - exactly 10s",
			attempt:  2,
			feedback: 10 * time.Second,
			want:     120 * time.Second, // (10s * 3) << 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextDelay(tt.attempt, tt.feedback)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNextDelay_SawtoothPattern(t *testing.T) {
	// Test that the delay pattern exhibits sawtooth behavior
	feedback := 50 * time.Millisecond

	var delays []time.Duration
	for attempt := range 20 {
		delays = append(delays, NextDelay(attempt, feedback))
	}

	// First few attempts should show exponential growth
	assert.True(t, delays[1] > delays[0]) // attempt 1 > attempt 0
	assert.True(t, delays[2] > delays[1]) // attempt 2 > attempt 1
	assert.True(t, delays[3] > delays[2]) // attempt 3 > attempt 2

	// At attempt 16, shift should wrap around, causing a "reset" in the exponential pattern
	// delay[16] should be significantly less than delay[15] due to shift reset
	assert.True(t, delays[16] < delays[15])
	// but still larger than delay[0] due to attempt multiplier
	assert.True(t, delays[16] > delays[0])
	// which is 17x larger than the initial delay
	assert.Equal(t, delays[0]*17, delays[16])
}
