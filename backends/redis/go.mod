module github.com/ajiwo/ratelimit/backends/redis

go 1.25.0

replace github.com/ajiwo/ratelimit v0.0.9 => ../..

require (
	github.com/ajiwo/ratelimit v0.0.9
	github.com/redis/go-redis/v9 v9.22.0
	github.com/stretchr/testify v1.12.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.44.0 // indirect
)
