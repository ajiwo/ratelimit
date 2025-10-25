package internal

type Config interface {
	GetKey() string
	GetCapacity() int
	GetLeakRate() float64
}
