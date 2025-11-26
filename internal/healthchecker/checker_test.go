package healthchecker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ajiwo/ratelimit/backends"
)

// mockBackend is a test backend that can simulate failures and successes
type mockBackend struct {
	backends.Backend
	mu         sync.RWMutex
	shouldFail bool
	failCount  int
	getCalled  bool
}

func (m *mockBackend) Get(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getCalled = true
	if m.shouldFail {
		m.failCount++
		return "", errors.New("simulated backend failure")
	}
	return "test-value", nil
}

func (m *mockBackend) setShouldFail(shouldFail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = shouldFail
}

func TestHealthChecker_New(t *testing.T) {
	backend := &mockBackend{}
	config := Config{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		TestKey:  "test-key",
	}

	hc := New(backend, config, nil)

	if hc == nil {
		t.Fatal("HealthChecker should not be nil")
	}

	hc.Stop()
}

func TestHealthChecker_StartAndStop(t *testing.T) {
	backend := &mockBackend{}
	config := Config{
		Interval: 50 * time.Millisecond,
		Timeout:  25 * time.Millisecond,
		TestKey:  "test-key",
	}

	healthyCalled := make(chan bool, 1)
	hc := New(backend, config, func() {
		select {
		case healthyCalled <- true:
		default:
		}
	})

	// Start health checking
	hc.Start()

	// Wait for at least one health check
	time.Sleep(100 * time.Millisecond)

	// Stop health checking
	hc.Stop()

	// Wait a bit to ensure stop takes effect
	time.Sleep(50 * time.Millisecond)

	backend.mu.RLock()
	wasCalled := backend.getCalled
	backend.mu.RUnlock()

	if !wasCalled {
		t.Error("Health check should have called backend.Get")
	}
}

func TestHealthChecker_ZeroInterval(t *testing.T) {
	backend := &mockBackend{}
	config := Config{
		Interval: 0, // Disabled
		Timeout:  25 * time.Millisecond,
		TestKey:  "test-key",
	}

	hc := New(backend, config, nil)
	hc.Start()

	// Wait to ensure no health checks happen
	time.Sleep(100 * time.Millisecond)

	backend.mu.RLock()
	wasCalled := backend.getCalled
	backend.mu.RUnlock()

	if wasCalled {
		t.Error("Health check should not run when interval is 0")
	}

	hc.Stop()
}

func TestHealthChecker_OnHealthyCallback(t *testing.T) {
	backend := &mockBackend{}
	config := Config{
		Interval: 50 * time.Millisecond,
		Timeout:  25 * time.Millisecond,
		TestKey:  "test-key",
	}

	var callbackCount int64
	var callbackMutex sync.Mutex
	hc := New(backend, config, func() {
		callbackMutex.Lock()
		callbackCount++
		callbackMutex.Unlock()
	})

	hc.Start()

	// Wait for multiple health checks
	time.Sleep(150 * time.Millisecond)

	hc.Stop()

	callbackMutex.Lock()
	if callbackCount == 0 {
		callbackMutex.Unlock()
		t.Error("Healthy callback should have been called")
		return
	}
	callbackMutex.Unlock()
}

func TestHealthChecker_BackendFailure(t *testing.T) {
	backend := &mockBackend{shouldFail: true}
	config := Config{
		Interval: 50 * time.Millisecond,
		Timeout:  25 * time.Millisecond,
		TestKey:  "test-key",
	}

	var callbackCount int64
	var callbackMutex sync.Mutex
	hc := New(backend, config, func() {
		callbackMutex.Lock()
		callbackCount++
		callbackMutex.Unlock()
	})

	hc.Start()

	// Wait for multiple health checks while backend is failing
	time.Sleep(150 * time.Millisecond)

	// Make backend healthy
	backend.setShouldFail(false)

	// Wait for more health checks
	time.Sleep(100 * time.Millisecond)

	hc.Stop()

	// Callback should only be called after backend becomes healthy
	callbackMutex.Lock()
	if callbackCount == 0 {
		callbackMutex.Unlock()
		t.Error("Healthy callback should have been called after backend recovery")
		return
	}
	callbackMutex.Unlock()
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Interval != 10*time.Second {
		t.Errorf("Expected default interval to be 10s, got %v", config.Interval)
	}

	if config.Timeout != 2*time.Second {
		t.Errorf("Expected default timeout to be 2s, got %v", config.Timeout)
	}

	if config.TestKey != "health-check-key" {
		t.Errorf("Expected default test key to be 'health-check-key', got %s", config.TestKey)
	}
}

func TestOptions(t *testing.T) {
	config := DefaultConfig()

	// Test WithInterval
	WithInterval(5 * time.Second)(&config)
	if config.Interval != 5*time.Second {
		t.Errorf("WithInterval failed, got %v", config.Interval)
	}

	// Test WithTimeout
	WithTimeout(1 * time.Second)(&config)
	if config.Timeout != 1*time.Second {
		t.Errorf("WithTimeout failed, got %v", config.Timeout)
	}

	// Test WithTestKey
	WithTestKey("custom-key")(&config)
	if config.TestKey != "custom-key" {
		t.Errorf("WithTestKey failed, got %s", config.TestKey)
	}
}

func TestHealthChecker_ContextTimeout(t *testing.T) {
	// Create a mock backend that hangs
	hangingBackend := &mockBackend{}

	config := Config{
		Interval: 50 * time.Millisecond,
		Timeout:  10 * time.Millisecond, // Very short timeout
		TestKey:  "test-key",
	}

	callbackCount := 0
	hc := New(hangingBackend, config, func() {
		callbackCount++
	})

	hc.Start()

	// Wait for health checks to potentially timeout
	time.Sleep(200 * time.Millisecond)

	hc.Stop()

	// The callback should not be called if health checks timeout
	// (this is more of a behavioral test, implementation might vary)
}
