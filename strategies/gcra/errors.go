package gcra

import "errors"

// ErrInvalidConfig is returned when the provided config is not of type gcra.Config.
var ErrInvalidConfig = errors.New("gcra strategy requires gcra.Config")
