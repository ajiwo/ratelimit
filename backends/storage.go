package backends

import (
	"context"
	"maps"
	"time"
)

// Storage defines the interface for storage operations
type Storage interface {
	// Get retrieves state data for the given key
	Get(ctx context.Context, key string) (*State, error)

	// Set stores state data for the given key with optional TTL
	Set(ctx context.Context, key string, state *State, ttl time.Duration) error

	// CompareAndSet atomically updates state if it matches expected state
	// Returns true if the update was successful, false if state changed
	CompareAndSet(ctx context.Context, key string, expected *State, new *State, ttl time.Duration) (bool, error)

	// Delete removes state data for the given key
	Delete(ctx context.Context, key string) error

	// Close releases any resources held by the storage backend
	Close() error
}

// State represents neutral storage data that can be used by any strategy
type State struct {
	// Counter stores numeric values (request counts, etc.)
	Counter int64 `json:"counter"`

	// Timestamps store time-based data (window start, last request, etc.)
	Timestamps map[string]time.Time `json:"timestamps"`

	// Values store floating-point values (tokens, rates, etc.)
	Values map[string]float64 `json:"values"`

	// Metadata stores additional strategy-specific data
	Metadata map[string]any `json:"metadata"`
}

// NewState creates a new empty state
func NewState() *State {
	return &State{
		Counter:    0,
		Timestamps: make(map[string]time.Time),
		Values:     make(map[string]float64),
		Metadata:   make(map[string]any),
	}
}

// WithCounter sets the counter value
func (s *State) WithCounter(value int64) *State {
	s.Counter = value
	return s
}

// WithTimestamp sets a timestamp value
func (s *State) WithTimestamp(key string, value time.Time) *State {
	s.Timestamps[key] = value
	return s
}

// WithValue sets a floating-point value
func (s *State) WithValue(key string, value float64) *State {
	s.Values[key] = value
	return s
}

// WithMetadata sets metadata
func (s *State) WithMetadata(key string, value any) *State {
	s.Metadata[key] = value
	return s
}

// GetTimestamp retrieves a timestamp value with default
func (s *State) GetTimestamp(key string, defaultValue time.Time) time.Time {
	if val, exists := s.Timestamps[key]; exists {
		return val
	}
	return defaultValue
}

// GetValue retrieves a floating-point value with default
func (s *State) GetValue(key string, defaultValue float64) float64 {
	if val, exists := s.Values[key]; exists {
		return val
	}
	return defaultValue
}

// GetMetadata retrieves metadata value with default
func (s *State) GetMetadata(key string, defaultValue any) any {
	if val, exists := s.Metadata[key]; exists {
		return val
	}
	return defaultValue
}

// Clone creates a copy of the state
func (s *State) Clone() *State {
	newState := NewState()
	newState.Counter = s.Counter

	// Copy timestamps
	maps.Copy(newState.Timestamps, s.Timestamps)

	// Copy values
	maps.Copy(newState.Values, s.Values)

	// Copy metadata (shallow copy for simplicity)
	maps.Copy(newState.Metadata, s.Metadata)

	return newState
}
