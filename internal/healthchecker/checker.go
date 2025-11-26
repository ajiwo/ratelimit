package healthchecker

import (
	"context"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// Checker monitors backend health and triggers recovery
type Checker struct {
	backend   backends.Backend
	config    Config
	stopChan  chan bool
	onHealthy func() // Callback when backend becomes healthy
}

// New creates a new health checker with the given backend and configuration
func New(backend backends.Backend, config Config, onHealthy func()) *Checker {
	return &Checker{
		backend:   backend,
		config:    config,
		stopChan:  make(chan bool),
		onHealthy: onHealthy,
	}
}

// Start begins background health monitoring
func (h *Checker) Start() {
	if h.config.Interval <= 0 {
		// Health checking disabled
		return
	}

	go func() {
		ticker := time.NewTicker(h.config.Interval)
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
func (h *Checker) Stop() {
	select {
	case h.stopChan <- true:
	default:
		// Channel already closed or stopped
	}
}

// checkHealth tests backend connectivity
func (h *Checker) checkHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
	defer cancel()

	testKey := h.config.TestKey
	if testKey == "" {
		testKey = "health-check-key"
	}

	// Try a simple Get operation to test connectivity
	_, err := h.backend.Get(ctx, testKey)
	if err == nil && h.onHealthy != nil {
		// Backend is healthy
		h.onHealthy()
	}
	// If there's an error, do nothing - caller will handle failures
}
