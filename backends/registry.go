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

// RedisConfig represents Redis backend configuration
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

// PostgresConfig represents PostgreSQL backend configuration
type PostgresConfig struct {
	ConnString string
	MaxConns   int32
	MinConns   int32
}
