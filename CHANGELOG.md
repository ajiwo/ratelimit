# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.0.5] - 2025-10-16

### Added
- **GCRA (Generic Cell Rate Algorithm)** strategy implementation for precise rate limiting with minimal jitter
- **Strategy Registry System** with plugin-based architecture for dynamic strategy discovery and creation
- **Strategy ID types** and capability-based validation for type-safe strategy management
- **Builder pattern** for Fixed Window configuration with `fixedwindow.NewConfig(key).AddQuota(name, limit, window).Build()`
- **Composite Strategy** for dual-strategy rate limiting with dedicated orchestration logic
- **Echo framework middleware example** demonstrating integration with web frameworks
- **Custom strategy composition example** showing how to build custom rate limiters with user-defined decision logic (AND, OR, priority-based, etc.)
- **Role-based strategy configurations** with Primary/Secondary role awareness
- **Strategy configuration packages** moved to dedicated packages for better modularity

### Changed
- **Terminology update**: "tier" renamed to "quota" throughout codebase for clarity
- **Strategy interface redesigned** to return `map[string]Result` for better multi-quota support
- **Dual-strategy orchestration** moved from main limiter to dedicated Composite Strategy
- **Strategy configurations** moved to individual strategy packages with `WithKey()` interface
- **Backend configuration** standardized across all storage backends
- **Memory backend** enhanced with automatic cleanup for stale entries
- **Strategy validation** replaced string-based type checks with capability-based validation
- **Result structure** unified across all strategies with consistent `map[string]Result` format
- **State serialization** optimized for improved performance and reduced storage overhead

### Removed
- **Strategy-specific configuration functions** in favor of generic strategy registry system
- **Per-key locking** from strategy layer (moved to memory backend)

### Fixed
- **Context cancellation** checks added to retry loops for better responsiveness
- **Strategy configuration** validation to ensure explicit strategy selection
- **Memory leaks** in memory backend through automatic cleanup implementation
- **Race conditions** in concurrent access scenarios through improved atomic operations

### Performance
- **Optimized state storage** format for strategies with compact serialization
- **Reduced memory overhead** through improved cleanup mechanisms

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
