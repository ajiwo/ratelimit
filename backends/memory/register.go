package memory

import (
	"github.com/ajiwo/ratelimit/backends"
)

func init() {
	backends.Register("memory", func(config any) (backends.Backend, error) {
		// Memory backend doesn't need configuration
		// Use New() which includes default 10-minute auto cleanup
		return New(), nil
	})
}
