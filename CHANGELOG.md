# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed
- **Breaking**: Move composite strategy implementation to internal package
  - Move strategies/composite/* files to internal/strategies/composite/*
  - Internal composite strategy packages are now private and not part of public API
  - Update import paths in main ratelimit and test files
  - Use "dual-strategy" terminology in public API and documentation
  - Keep "composite strategy" terminology for internal/maintainer documentation

## [0.0.8] - 2025-11-19

### Added
- GCRA strategy support in secondary role for dual-mode rate limiting
- `NewWithClient` function for remote backends to accept pre-configured clients
- Benchmark tests for rate limiting 

### Performance
- Optimized Redis `CheckAndSet` operations using cached Lua script

### Changed
- Simplified dual-strategy benchmark tests with configuration-driven approach
- Enhanced context error handling in allow functions
- Simplified state retrieval in fixed window `allowReadOnly` operations

### Fixed
- Fixed memory backend cleanup using proper Clear() method for sync.Map
- Prevented zero rate limit values in concurrent access tests
- Adjusted concurrent test settings for CI environment stability

### Removed
- Unreachable conditional logic in `buildStrategyConfig`
- Unreachable dead code from rate limiting strategies
- Unused error functions from strategy packages
- Duplicate basic refill test in token bucket
- Outdated custom example

## [0.0.7] - 2025-11-07

### Added
- Direct composite strategy usage example demonstrating standalone strategy composition
- General purpose `utils.SleepOrWait` helper function for context-aware long waiting or short sleeping

### Changed
- **Backend interface** `Set` and `CheckAndSet` methods now use `string` values instead of `any` type, removing type assertions
- **Fixed Window strategy**
  - improved with combined state approach storing all quotas together under a single key with atomic operations
  - limited the quota number to maximum 8 quotas per key
- **Composite strategy** redesigned with atomic operations using single composite state encoding instead of separate primary/secondary keys
- **Strategy type names** simplified to `StrategyConfig` -> `Config`, `StrategyID` -> `ID`, `StrategyRole` -> `Role` throughout codebase
- **Composite strategy** moved to dedicated strategies/composite subpackage
- **Retry backoff** enhanced with centralized NextDelay function and improved context cancellation handling
- **String Builder pooling** centralized in dedicated utils/builderpool package for better memory management
- **Strategy data format** replaced generic "v2|" prefix with unique hex identifiers for each strategy type

### Performance
- **Optimized retry parameters** - adjusted delay ranges and reduced default max retries from 300 to 30 with improved backoff strategy

### Fixed
- **Context cancellation handling** improved in retry loops with proper short/long delay distinction
- **Retry duration overflow** capped exponential backoff growth with modulo 8

## [0.0.6] - 2025-10-26

### Added
- `RateLimiter.Peek` to inspect quota status without consuming allowance while still returning an overall allowed flag
- `WithMaxRetries` option and `StrategyConfig.WithMaxRetries` support to tune optimistic locking retries across all strategies

### Changed
- Rate limiter call sites now pass `context.Context` explicitly along with an `AccessOptions` struct, replacing the previous variadic functional access options
- Strategy interface replaces `GetResult` with `Peek`, and strategy packages adopt shared internal modules for reusable state management
- Introduced `strategies.Results` type alias to standardize result maps across the codebase

### Fixed
- `Config.Validate` now applies the same key validation logic as dynamic keys, preventing invalid base keys from being accepted
- Composite strategy derives distinct `:p`/`:s` suffixed keys for primary and secondary configurations to prevent key collisions
- Memory backend operations bail out early when contexts are cancelled instead of blocking on per-key locks
- Redis backend read/write helpers wrap underlying errors with contextual messages for easier diagnostics

### Performance
- Rate limiter and composite strategies reuse pooled string builders when constructing storage keys to reduce allocations
- Memory backend reuses mutexes via a sync.Pool to cut per-key allocation overhead during high churn

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
