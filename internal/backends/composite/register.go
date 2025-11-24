package composite

import (
	"github.com/ajiwo/ratelimit/backends"
)

func init() {
	backends.Register("composite", func(config any) (backends.Backend, error) {
		compositeConfig, ok := config.(Config)
		if !ok {
			return nil, backends.ErrInvalidConfig
		}

		// Set defaults and validate
		compositeConfig.SetDefaults()
		if err := compositeConfig.Validate(); err != nil {
			return nil, backends.ErrInvalidConfig
		}

		return New(compositeConfig)
	})
}
