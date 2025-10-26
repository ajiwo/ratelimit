package internal

import (
	"context"
	"math"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

type AllowMode int

const (
	ReadOnly AllowMode = iota
	TryUpdate
)

type Result struct {
	Allowed      bool
	Remaining    int
	Reset        time.Time
	stateUpdated bool
}

func Allow(ctx context.Context, storage backends.Backend, config Config, mode AllowMode) (Result, error) {
	now := time.Now()
	capacity := float64(config.GetBurstSize())
	refillRate := config.GetRefillRate()

	if mode == ReadOnly {
		return allowReadOnly(ctx, storage, config, now, capacity, refillRate)
	}

	return allowTryAndUpdate(ctx, storage, config, now, capacity, refillRate)
}

func allowReadOnly(ctx context.Context, storage backends.Backend, tbConfig Config, now time.Time, capacity float64, refillRate float64) (Result, error) {
	data, err := storage.Get(ctx, tbConfig.GetKey())
	if err != nil {
		return Result{}, NewStateRetrievalError(err)
	}

	if data == "" {
		return Result{
			Allowed:      true,
			Remaining:    tbConfig.GetBurstSize(),
			Reset:        now,
			stateUpdated: false,
		}, nil
	}

	bucket, ok := decodeState(data)
	if !ok {
		return Result{}, ErrStateParsing
	}

	timeElapsed := now.Sub(bucket.LastRefill).Seconds()
	tokensToAdd := timeElapsed * refillRate
	bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, capacity)
	bucket.LastRefill = now

	remaining := max(int(bucket.Tokens), 0)

	return Result{
		Allowed:      remaining > 0,
		Remaining:    remaining,
		Reset:        now,
		stateUpdated: false,
	}, nil
}

func allowTryAndUpdate(ctx context.Context, storage backends.Backend, tbConfig Config, now time.Time, capacity float64, refillRate float64) (Result, error) {
	maxRetries := tbConfig.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = strategies.DefaultMaxRetries
	}

	for attempt := range maxRetries {
		if ctx.Err() != nil {
			return Result{}, NewContextCancelledError(ctx.Err())
		}

		data, err := storage.Get(ctx, tbConfig.GetKey())
		if err != nil {
			return Result{}, NewStateRetrievalError(err)
		}

		var bucket TokenBucket
		var oldValue any
		if data == "" {
			bucket = TokenBucket{
				Tokens:     capacity,
				LastRefill: now,
			}
			oldValue = nil
		} else {
			if b, ok := decodeState(data); ok {
				bucket = b
			} else {
				return Result{}, ErrStateParsing
			}
			oldValue = data

			elapsed := now.Sub(bucket.LastRefill)
			tokensToAdd := float64(elapsed.Nanoseconds()) * refillRate / 1e9
			bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, capacity)
			bucket.LastRefill = now
		}

		allowed := math.Floor(bucket.Tokens) >= 1.0

		if allowed {
			bucket.Tokens -= 1.0
			remaining := max(int(bucket.Tokens), 0)

			newValue := encodeState(bucket)
			expiration := strategies.CalcExpiration(tbConfig.GetBurstSize(), tbConfig.GetRefillRate())

			success, err := storage.CheckAndSet(ctx, tbConfig.GetKey(), oldValue, newValue, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}

			if success {
				return Result{
					Allowed:      true,
					Remaining:    remaining,
					Reset:        now,
					stateUpdated: true,
				}, nil
			}

			if attempt < maxRetries-1 {
				time.Sleep((19 * time.Nanosecond) << time.Duration(attempt))
				continue
			}
			break
		}

		remaining := max(int(bucket.Tokens), 0)
		bucketData := encodeState(bucket)
		expiration := strategies.CalcExpiration(tbConfig.GetBurstSize(), tbConfig.GetRefillRate())

		if oldValue == nil {
			_, err := storage.CheckAndSet(ctx, tbConfig.GetKey(), oldValue, bucketData, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}
		}

		return Result{
			Allowed:      false,
			Remaining:    remaining,
			Reset:        calculateResetTime(now, bucket, refillRate),
			stateUpdated: oldValue == nil,
		}, nil
	}

	return Result{}, ErrConcurrentAccess
}

func calculateResetTime(now time.Time, bucket TokenBucket, refillRate float64) time.Time {
	if bucket.Tokens >= 1.0 {
		return now
	}

	tokensNeeded := 1.0 - bucket.Tokens
	if tokensNeeded <= 0 {
		return now
	}

	timeToRefillSeconds := tokensNeeded / refillRate
	return now.Add(time.Duration(timeToRefillSeconds * float64(time.Second)))
}
