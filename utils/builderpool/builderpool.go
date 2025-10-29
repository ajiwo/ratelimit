package builderpool

import (
	"strings"
	"sync"
)

var pool = sync.Pool{
	New: func() any {
		return &strings.Builder{}
	},
}

func Get() *strings.Builder {
	sb := pool.Get().(*strings.Builder)
	sb.Reset()
	sb.Grow(64)
	return sb
}

func Put(sb *strings.Builder) {
	pool.Put(sb)
}
