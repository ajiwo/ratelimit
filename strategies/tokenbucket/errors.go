package tokenbucket

import "errors"

// ErrInvalidConfig is returned when the provided config is not of type tokenbucket.Config.
var ErrInvalidConfig = errors.New("token bucket strategy requires tokenbucket.Config")
