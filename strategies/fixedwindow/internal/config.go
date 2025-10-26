package internal

import "time"

type Config interface {
	GetKey() string
	GetQuotas() map[string]Quota
	MaxRetries() int
}

type Quota struct {
	Limit  int
	Window time.Duration
}
