package strategies

import "time"

type FixedWindowConfig struct {
	Key    string
	Limit  int
	Window time.Duration
}

type LeakyBucketConfig struct {
	Key      string
	Capacity int     // Maximum requests the bucket can hold
	LeakRate float64 // Requests to process per second
}

type TokenBucketConfig struct {
	Key        string
	BurstSize  int     // Maximum tokens the bucket can hold
	RefillRate float64 // Tokens to add per second
}
