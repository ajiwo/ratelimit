# Changelog

All notable changes to this project will be documented in this file.

## [0.0.4] - 2025-10-06

### Added
- Dual-strategy rate limiting with primary fixed-window (multi-tier) and secondary bucket-based strategies for combined rate limiting and traffic shaping
- `strategies.AllowWithResult` method to return detailed rate limit statistics including Total and Used fields
- `backends.CheckAndSet` atomic operation to backend interface for atomic operations
- Strategy-specific configuration system with `ratelimit.StrategyConfig` interface and implementations 

### Changed
- Fixed Window strategy is now the only multi-tier capable strategy, as multi-tier bucket-based strategies were redundant
- Removed multi-tier support from bucket-based strategies (leaky/token bucket) for cleaner architecture
- Redesigned MultiTierConfig to use `ratelimit.PrimaryConfig` and `ratelimit.SecondaryConfig` fields instead of field-based strategy configuration
- Moved validation logic from centralized validator to individual configuration structs for better modularity
- Updated all examples to use AllowWithResult for better statistics
- Refactored backend configuration with modular constructors standardized to New() function
- Simplified MultiTierLimiter.Allow to delegate to AllowWithResult

### Fixed
- Prevent panic when closing MultiTierLimiter with nil or closed channel

### Deprecated
- `strategies.Allow` methods in favor of `strategies.AllowWithResult` for better performance and detailed results

## [0.0.3] - 2025-09-28

### Added
- PostgreSQL backend storage support for persistent rate limiting state.
- Dynamic key support for multi-tenant rate limiting scenarios.
- Support for period (.) and at (@) symbols in rate limit keys.
- PostgreSQL service integration in CI pipeline.

### Changed
- Replaced context parameters with functional options.
- Restructured backends into modular plugin architecture.
- Optimized key construction and validation with precomputed character arrays.
- Updated examples to demonstrate functional options refactor and Redis pool size configuration.
- Improved backend error handling across all storage implementations.
- Standardized backend constructors to use `New()` function across all backends.
- Decoupled configuration types from registry to individual backend packages.
- Replaced backend-specific option functions `WithMemoryBackend`, `WithRedisBackend`, `WithPostgresBackend`
  with generic `WithBackend()` option.

### Performance
- Replaced JSON serialization with compact custom format for rate limiter state, improving serialization efficiency.

## [0.0.2] - 2025-09-19

### Added
- Automatic cleanup for stale locks.
- GitHub Actions workflow for continuous integration.
- Leaky bucket rate limiting strategy.
- Multi-tier rate limiting with configurable strategies.
- Token bucket rate limiting strategy.
- Fixed window rate limiting strategy.
- Tests for multi-tier rate limiting.

### Changed
- Redesigned the storage and strategy interfaces for improved extensibility and simpler implementation of new strategies.
- Updated documentation and examples to reflect the new API.
- Updated CI configuration to use golangci-lint.

### Fixed
- Improved memory backend cleanup race condition handling.

## [0.0.1] - 2025-09-13

### Added
- Initial version of the rate limiting library.
