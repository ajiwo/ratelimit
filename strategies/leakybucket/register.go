package leakybucket

import (
	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

func init() {
	strategies.Register(strategies.StrategyLeakyBucket, func(storage backends.Backend) strategies.Strategy {
		return New(storage)
	})
}
