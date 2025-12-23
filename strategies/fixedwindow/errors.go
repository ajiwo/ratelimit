package fixedwindow

import "errors"

// ErrInvalidConfig is returned when the provided config is not of type fixedwindow.Config.
var ErrInvalidConfig = errors.New("fixed window strategy requires fixedwindow.Config")
