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
