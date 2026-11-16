module github.com/ajiwo/ratelimit/tests

go 1.25.0

replace github.com/ajiwo/ratelimit v0.0.9 => ../

replace github.com/ajiwo/ratelimit/backends/postgres v0.0.9 => ../backends/postgres

replace github.com/ajiwo/ratelimit/backends/redis v0.0.9 => ../backends/redis

require (
	github.com/ajiwo/ratelimit v0.0.9
	github.com/ajiwo/ratelimit/backends/postgres v0.0.9
	github.com/ajiwo/ratelimit/backends/redis v0.0.9
	github.com/stretchr/testify v1.12.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.13.0 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
