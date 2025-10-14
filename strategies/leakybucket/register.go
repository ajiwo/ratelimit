package leakybucket

import (
	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

func init() {
	strategies.Register("leaky_bucket", func(storage backends.Backend) strategies.Strategy {
		return New(storage)
	})
}
