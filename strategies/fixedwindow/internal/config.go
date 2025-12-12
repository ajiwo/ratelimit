package internal

import "time"

// MaxQuota defines the maximum number of quota configurations allowed
// in a fixed window rate limiter.
const MaxQuota = 8

type Config interface {
	GetKey() string
	GetQuotas() []Quota
	MaxRetries() int
}

type Quota struct {
	Name   string
	Limit  int
	Window time.Duration
}
