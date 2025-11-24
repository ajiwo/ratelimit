package composite

import (
	"context"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// CheckerConfig holds configuration for health checking
type CheckerConfig struct {
	Interval time.Duration // Health check frequency
	Timeout  time.Duration // Health check timeout
	TestKey  string        // Key for health check operations
}

// healthChecker monitors backend health and triggers recovery
type healthChecker struct {
	primary   backends.Backend
	config    CheckerConfig
	stopChan  chan bool
	onHealthy func() // Callback when primary becomes healthy
}

// newHealthChecker creates a new health checker
func newHealthChecker(primary backends.Backend, config CheckerConfig, onHealthy func()) *healthChecker {
	return &healthChecker{
		primary:   primary,
		config:    config,
		stopChan:  make(chan bool),
		onHealthy: onHealthy,
	}
}

// Start begins background health monitoring
func (h *healthChecker) Start() {
	if h.config.Interval <= 0 {
		// Health checking disabled
		return
	}

	ticker := time.NewTicker(h.config.Interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.checkHealth()
			case <-h.stopChan:
				return
			}
		}
	}()
}

// Stop stops health monitoring
func (h *healthChecker) Stop() {
	select {
	case h.stopChan <- true:
	default:
		// Channel already closed or stopped
	}
}

// checkHealth tests primary backend connectivity
func (h *healthChecker) checkHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
	defer cancel()

	testKey := h.config.TestKey
	if testKey == "" {
		testKey = "health-check-key"
	}

	// Try a simple Get operation to test connectivity
	_, err := h.primary.Get(ctx, testKey)
	if err == nil && h.onHealthy != nil {
		// Primary is healthy
		h.onHealthy()
	}
	// If there's an error, do nothing - circuit breaker will handle failures
}
