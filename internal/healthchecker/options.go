package healthchecker

import "time"

// Option configures the HealthChecker
type Option func(*Config)

// WithInterval sets the health check interval
func WithInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.Interval = interval
	}
}

// WithTimeout sets the health check timeout
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithTestKey sets the key used for health check operations
func WithTestKey(testKey string) Option {
	return func(c *Config) {
		c.TestKey = testKey
	}
}
