package strategies

import "github.com/ajiwo/ratelimit/backends"

// StrategyFactory creates a strategy instance using a backend
// The returned Strategy should be ready to use with an appropriate StrategyConfig
// provided at Allow/GetResult time.
type StrategyFactory func(storage backends.Backend) Strategy

var registeredStrategies = make(map[string]StrategyFactory)

// Register registers a strategy factory under a unique name
func Register(name string, factory StrategyFactory) {
	registeredStrategies[name] = factory
}

// Create creates a strategy instance by name using the provided backend
func Create(name string, storage backends.Backend) (Strategy, error) {
	factory, ok := registeredStrategies[name]
	if !ok {
		return nil, ErrStrategyNotFound
	}
	return factory(storage), nil
}
