package leakybucket

import "errors"

// ErrInvalidConfig is returned when the provided config is not of type leakybucket.Config.
var ErrInvalidConfig = errors.New("leaky bucket strategy requires leakybucket.Config")
