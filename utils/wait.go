package utils

import (
	"context"
	"time"
)

// SleepOrWait provides context-aware long waiting or short sleeping.
//
// For delay <= threshold, it uses `time.Sleep` directly, ignoring context cancellation.
// For delay > threshold, it respects context cancellation.
func SleepOrWait(ctx context.Context, delay time.Duration, threshold time.Duration) error {
	if delay <= threshold {
		time.Sleep(delay)
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
