package internal

type Config interface {
	GetKey() string
	GetBurst() int
	GetRate() float64
	MaxRetries() int
}
