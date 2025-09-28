package backends

// BackendFactory creates a backend instance with optional configuration
type BackendFactory func(config any) (Backend, error)

// registeredBackends holds all registered backend factories
var registeredBackends = make(map[string]BackendFactory)

// Register registers a backend factory function
func Register(name string, factory BackendFactory) {
	registeredBackends[name] = factory
}

// Create creates a backend instance with optional configuration
func Create(name string, config any) (Backend, error) {
	factory, ok := registeredBackends[name]
	if !ok {
		return nil, ErrBackendNotFound
	}
	return factory(config)
}
