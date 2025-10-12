module github.com/ajiwo/ratelimit/examples

go 1.25.0

replace github.com/ajiwo/ratelimit v0.0.4 => ../

replace github.com/ajiwo/ratelimit/backends/postgres v0.0.4 => ../backends/postgres

replace github.com/ajiwo/ratelimit/backends/redis v0.0.4 => ../backends/redis

require github.com/ajiwo/ratelimit v0.0.4
