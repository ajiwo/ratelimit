package internal

import "time"

type Config interface {
	GetKey() string
	GetQuotas() map[string]Quota
}

type Quota struct {
	Limit  int
	Window time.Duration
}
