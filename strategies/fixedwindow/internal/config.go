package internal

import "time"

// MaxQuota defines the maximum number of quota configurations allowed
// in a fixed window rate limiter.
const MaxQuota = 8

type Config interface {
	GetKey() string
	GetQuotas() map[string]Quota
	MaxRetries() int
}

type Quota struct {
	Limit  int
	Window time.Duration
}
