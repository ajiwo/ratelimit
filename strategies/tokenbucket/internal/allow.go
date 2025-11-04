package internal

import (
	"context"
	"math"
	"time"

	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
	"github.com/ajiwo/ratelimit/utils"
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

type parameter struct {
	burstSize  int
	capacity   float64
	key        string
	now        time.Time
	maxRetries int
	refillRate float64
	storage    backends.Backend
}

func Allow(
	ctx context.Context,
	storage backends.Backend,
	config Config,
	mode AllowMode,

) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, NewContextCanceledError(ctx.Err())
	}

	maxRetries := config.MaxRetries()
	if maxRetries <= 0 {
		maxRetries = strategies.DefaultMaxRetries
	}

	p := &parameter{
		burstSize:  config.GetBurstSize(),
		capacity:   float64(config.GetBurstSize()),
		key:        config.GetKey(),
		maxRetries: maxRetries,
		now:        time.Now(),
		refillRate: config.GetRefillRate(),
		storage:    storage,
	}

	if mode == ReadOnly {
		return p.allowReadOnly(ctx)
	}

	return p.allowTryAndUpdate(ctx)
}

func (p *parameter) allowReadOnly(ctx context.Context) (Result, error) {
	data, err := p.storage.Get(ctx, p.key)
	if err != nil {
		return Result{}, NewStateRetrievalError(err)
	}

	if data == "" {
		return Result{
			Allowed:      true,
			Remaining:    p.burstSize,
			Reset:        p.now,
			stateUpdated: false,
		}, nil
	}

	bucket, ok := decodeState(data)
	if !ok {
		return Result{}, ErrStateParsing
	}

	timeElapsed := p.now.Sub(bucket.LastRefill).Seconds()
	tokensToAdd := timeElapsed * p.refillRate
	bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, p.capacity)
	bucket.LastRefill = p.now

	remaining := max(int(bucket.Tokens), 0)

	return Result{
		Allowed:      remaining > 0,
		Remaining:    remaining,
		Reset:        p.now,
		stateUpdated: false,
	}, nil
}

func (p *parameter) allowTryAndUpdate(ctx context.Context) (Result, error) {
	for attempt := range p.maxRetries {
		if ctx.Err() != nil {
			return Result{}, NewContextCanceledError(ctx.Err())
		}

		data, err := p.storage.Get(ctx, p.key)
		if err != nil {
			return Result{}, NewStateRetrievalError(err)
		}

		var bucket TokenBucket
		var oldValue string
		if data == "" {
			bucket = TokenBucket{
				Tokens:     p.capacity,
				LastRefill: p.now,
			}
			oldValue = ""
		} else {
			if b, ok := decodeState(data); ok {
				bucket = b
			} else {
				return Result{}, ErrStateParsing
			}
			oldValue = data

			elapsed := p.now.Sub(bucket.LastRefill)
			tokensToAdd := float64(elapsed.Nanoseconds()) * p.refillRate / 1e9
			bucket.Tokens = math.Min(bucket.Tokens+tokensToAdd, p.capacity)
			bucket.LastRefill = p.now
		}

		allowed := math.Floor(bucket.Tokens) >= 1.0

		if allowed {
			beforeCAS := time.Now()
			bucket.Tokens -= 1.0
			remaining := max(int(bucket.Tokens), 0)

			newValue := encodeState(bucket)
			expiration := strategies.CalcExpiration(p.burstSize, p.refillRate)

			success, err := p.storage.CheckAndSet(ctx, p.key, oldValue, newValue, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}

			if success {
				return Result{
					Allowed:      true,
					Remaining:    remaining,
					Reset:        p.now,
					stateUpdated: true,
				}, nil
			}

			feedback := time.Since(beforeCAS)
			delay := strategies.NextDelay(attempt, feedback)

			if attempt < p.maxRetries-1 {
				if err := utils.SleepOrWait(ctx, delay, 500*time.Millisecond); err != nil {
					return Result{}, NewContextCanceledError(err)
				}
				continue
			}
			break
		}

		remaining := max(int(bucket.Tokens), 0)
		bucketData := encodeState(bucket)
		expiration := strategies.CalcExpiration(p.burstSize, p.refillRate)

		if oldValue == "" {
			_, err := p.storage.CheckAndSet(ctx, p.key, oldValue, bucketData, expiration)
			if err != nil {
				return Result{}, NewStateSaveError(err)
			}
		}

		return Result{
			Allowed:      false,
			Remaining:    remaining,
			Reset:        calculateResetTime(p.now, bucket, p.refillRate),
			stateUpdated: oldValue == "",
		}, nil
	}

	return Result{}, ErrConcurrentAccess
}

func calculateResetTime(
	now time.Time,
	bucket TokenBucket,
	refillRate float64,

) time.Time {
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
