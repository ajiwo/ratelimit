# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- Detailed statistics for all rate limiting strategies including remaining capacity and reset times.
- Multi-tier rate limiting with per-tier result tracking.
- Secondary rate limiting strategy support for more flexible configurations
- `AllowWithResult` method to return detailed rate limit statistics with Total and Used fields
- TierResult with additional fields (Total, Used) for comprehensive rate limit information

### Changed
- Refactor: Remove tiers from bucket strategy options and update logic
- Refactor: Simplify MultiTierLimiter.Allow to delegate to AllowWithResult
- Refactor: Modularize backend configuration and constructors, standardizing to New() function

### Fixed
- Prevent panic when closing MultiTierLimiter with nil or closed channel
- Update LeakyBucket strategy's AllowWithResult method logic

### Deprecated
- Allow methods in favor of AllowWithResult for better performance and detailed results

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
