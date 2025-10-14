package fixedwindow

import (
	"github.com/ajiwo/ratelimit/backends"
	"github.com/ajiwo/ratelimit/strategies"
)

func init() {
	strategies.Register("fixed_window", func(storage backends.Backend) strategies.Strategy {
		return New(storage)
	})
}
