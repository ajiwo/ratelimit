package internal

type Config interface {
	GetKey() string
	GetBurstSize() int
	GetRefillRate() float64
	MaxRetries() int
}
