package strategies

import "time"

type Results map[string]Result

// Result represents the result of a rate limiting check
type Result struct {
	Allowed   bool      // Whether the request is allowed
	Remaining int       // Remaining requests in the current window
	Reset     time.Time // When the current window resets
}

// Default returns the result for the "default" quota.
//
// This is useful for single-quota scenarios where you only have one quota.
func (r Results) Default() Result {
	return r["default"]
}

func (r Results) PrimaryDefault() Result {
	return r["primary_default"]
}

func (r Results) SecondaryDefault() Result {
	return r["secondary_default"]
}

// Quota returns the result for the specified quota name.
//
// If the quota doesn't exist, returns a zero Result (Allowed: false, Remaining: 0, Reset: zero time).
func (r Results) Quota(name string) Result {
	if result, exists := r[name]; exists {
		return result
	}
	return Result{}
}

func (r Results) Primary(name string) Result {
	return r.Quota("primary_" + name)
}

func (r Results) Secondary(name string) Result {
	return r.Quota("secondary_" + name)
}

// First returns the first result in the map.
//
// Useful when you have multiple quotas but just need any one of them.
// Note: Map iteration order is not guaranteed in Go, so use with caution.
func (r Results) First() Result {
	for _, result := range r {
		return result
	}
	return Result{}
}

// HasQuota checks if a quota with the given name exists in the results.
func (r Results) HasQuota(name string) bool {
	_, exists := r[name]
	return exists
}

// AllAllowed returns true if all quotas in the results are allowed.
func (r Results) AllAllowed() bool {
	for _, result := range r {
		if !result.Allowed {
			return false
		}
	}
	return true
}

// AnyAllowed returns true if any quota in the results is allowed.
func (r Results) AnyAllowed() bool {
	for _, result := range r {
		if result.Allowed {
			return true
		}
	}
	return false
}

// Len returns the number of quotas in the results.
func (r Results) Len() int {
	return len(r)
}
