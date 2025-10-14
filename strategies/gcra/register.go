package gcra

import (
	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

func init() {
	strategies.Register("gcra", func(storage backends.Backend) strategies.Strategy {
		return New(storage)
	})
}
