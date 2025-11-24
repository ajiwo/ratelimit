package composite

import (
	"sync/atomic"
	"time"
)

// breakerState represents the circuit breaker state
type breakerState int32

const (
	stateClosed breakerState = iota
	stateHalfOpen
	stateOpen
)

// BreakerConfig holds configuration for circuit breaker
type BreakerConfig struct {
	FailureThreshold int32         // Number of failures before tripping
	RecoveryTimeout  time.Duration // Time to wait before trying primary again
}

// circuitBreaker implements circuit breaker pattern with 3 states using atomic operations
type circuitBreaker struct {
	config       BreakerConfig // meant to be read-only internally, never mutate this
	state        int32         // atomic, stores State value
	failureCount int32         // atomic failure counter
	openedAt     int64         // atomic, stores nanoseconds since Unix epoch
}

// newCircuitBreaker creates a new circuit breaker
func newCircuitBreaker(config BreakerConfig) *circuitBreaker {
	return &circuitBreaker{
		config: config,
		state:  int32(stateClosed),
	}
}

// ShouldTrip determines if circuit should trip based on error
func (cb *circuitBreaker) ShouldTrip(err error) bool {
	if err == nil {
		return false
	}

	// Atomically increment failure count
	newCount := atomic.AddInt32(&cb.failureCount, 1)

	// Check if we've reached the failure threshold
	if newCount >= cb.config.FailureThreshold {
		cb.Open()
		return true
	}
	return false
}

// IsOpen returns true if circuit is open (should use secondary backend)
func (cb *circuitBreaker) IsOpen() bool {
	currentState := breakerState(atomic.LoadInt32(&cb.state))

	switch currentState {
	case stateOpen:
		// Check if recovery timeout has passed
		openedAtNano := atomic.LoadInt64(&cb.openedAt)
		if time.Since(time.Unix(0, openedAtNano)) >= cb.config.RecoveryTimeout {
			// Try to transition to HALF-OPEN state
			if atomic.CompareAndSwapInt32(&cb.state, int32(stateOpen), int32(stateHalfOpen)) {
				return false
			}
		}
		return true
	case stateHalfOpen:
		return false
	default: // StateClosed
		return false
	}
}

// Open trips the circuit breaker to OPEN state
func (cb *circuitBreaker) Open() {
	atomic.StoreInt32(&cb.state, int32(stateOpen))
	atomic.StoreInt64(&cb.openedAt, time.Now().UnixNano())
}

// Close resets the circuit breaker to CLOSED state
func (cb *circuitBreaker) Close() {
	atomic.StoreInt32(&cb.state, int32(stateClosed))
	atomic.StoreInt32(&cb.failureCount, 0)
}

// GetState returns current circuit breaker state
func (cb *circuitBreaker) GetState() breakerState {
	return breakerState(atomic.LoadInt32(&cb.state))
}
