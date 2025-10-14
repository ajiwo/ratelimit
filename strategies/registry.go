package strategies

import "github.com/ajiwo/ratelimit/backends"

// StrategyFactory creates a strategy instance using a backend
// The returned Strategy should be ready to use with an appropriate StrategyConfig
// provided at Allow/GetResult time.
type StrategyFactory func(storage backends.Backend) Strategy

var registeredStrategies = make(map[StrategyID]StrategyFactory)

// Register registers a strategy factory under a unique ID
func Register(id StrategyID, factory StrategyFactory) {
	registeredStrategies[id] = factory
}

// Create creates a strategy instance by ID using the provided backend
func Create(id StrategyID, storage backends.Backend) (Strategy, error) {
	factory, ok := registeredStrategies[id]
	if !ok {
		return nil, ErrStrategyNotFound
	}
	return factory(storage), nil
}
