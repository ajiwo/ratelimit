package internal

import (
	"context"

	"github.com/ajiwo/ratelimit/backends"
)

func Reset(ctx context.Context, config Config, backend backends.Backend) error {
	key := config.GetKey()
	return backend.Delete(ctx, key)
}
