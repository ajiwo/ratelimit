# Redis Lua Scripts

This directory contains Lua scripts that are embedded at compile time for atomic Redis operations in the rate limiting system.

## Scripts

### `check_and_increment.lua`
- **Purpose**: Atomic check-and-increment with limit enforcement
- **Usage**: Fixed Window rate limiting strategy
- **Parameters**:
  - `KEYS[1]`: rate limit key
  - `ARGV[1]`: window_seconds
  - `ARGV[2]`: current timestamp
  - `ARGV[3]`: limit (max requests allowed)
- **Returns**: `{count, incremented}` where incremented is 1 if successful, 0 if limit exceeded

### `check_and_consume_token.lua`
- **Purpose**: Atomic token bucket operations with refill calculations
- **Usage**: Token Bucket rate limiting strategy
- **Parameters**:
  - `KEYS[1]`: rate limit key
  - `ARGV[1]`: bucket_size
  - `ARGV[2]`: refill_rate_seconds
  - `ARGV[3]`: refill_amount
  - `ARGV[4]`: window_seconds (for TTL)
  - `ARGV[5]`: current timestamp
- **Returns**: `{tokens, consumed, requests}` where consumed is 1 if token was consumed, 0 if insufficient tokens

## Data Structure

All scripts work with a common JSON data structure stored in Redis:

```json
{
  "count": 5,
  "window_start": 1640995200,
  "last_request": 1640995205,
  "tokens": 7.5,
  "last_refill": 1640995205,
  "metadata": {}
}
```

- `count`: Number of requests in current window
- `window_start`: Unix timestamp when current window started
- `last_request`: Unix timestamp of last request
- `tokens`: Current number of tokens (for token bucket)
- `last_refill`: Unix timestamp of last token refill
- `metadata`: Additional strategy-specific data

## TTL Management

All scripts automatically set TTL (Time To Live) on Redis keys:
- TTL = `window_seconds * 2` for safety margin
- Prevents memory leaks from abandoned rate limit keys
- Automatically cleans up expired data
