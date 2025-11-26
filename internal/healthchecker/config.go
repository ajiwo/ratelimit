package healthchecker

import "time"

// Config holds configuration for health checking
type Config struct {
	Interval time.Duration // Health check frequency
	Timeout  time.Duration // Health check timeout
	TestKey  string        // Key for health check operations
}

// DefaultConfig returns a health checker config with sensible defaults
func DefaultConfig() Config {
	return Config{
		Interval: 10 * time.Second,
		Timeout:  2 * time.Second,
		TestKey:  "health-check-key",
	}
}
