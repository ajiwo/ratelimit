package composite

import (
	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

func init() {
	strategies.Register(strategies.StrategyComposite, func(storage backends.Backend) strategies.Strategy {
		return &Strategy{
			storage: storage,
		}
	})
}
