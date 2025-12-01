# Ratelimit Data Format Documentation

This document describes the data format used by each rate limiting strategy. All formats use a 2-character header for identification and versioning, and `|` character as field separator.

## Header Format

**Format:** `AB`
- **A**: 1-digit hexadecimal strategy ID (see `strategies/config.go` lines 11-16)
- **B**: 1-digit hexadecimal internal version of the data format


### Strategy ID Mapping

| Strategy | ID (Decimal) | ID (Hex) |
|----------|--------------|----------|
| Token Bucket | 1 | 0x1 |
| Fixed Window | 2 | 0x2 |
| Leaky Bucket | 3 | 0x3 |
| GCRA | 4 | 0x4 |
| Composite | 5 | 0x5 |

---

## 1. Token Bucket Strategy (Header: `12`)

**Version:** 2 (0x2)
**Strategy ID:** 1 (0x1)
**Format:** `12|tokens|lastRefill_unix_nano`

### Data Structure
```go
type TokenBucket struct {
    Tokens     float64   // Available tokens (can be fractional)
    LastRefill time.Time // Last refill timestamp
}
```

### Format Breakdown
- `12`: Header (version 2, Token Bucket)
- `tokens`: Current token count (float64, format 'g')
- `lastRefill`: Last refill time as Unix nanoseconds (int64)

### Example
```
12|8.5|1761884055342794596
```
Decoded:
- 8.5 tokens available
- Last refill at 1761884055342794596 ns

### Key Characteristics
- Tokens as float64 (supports fractional values)
- Single logical limit (no multiple quotas)
- Burst = initial token count

---

## 2. Fixed Window Strategy (Header: `23`)

**Version:** 3 (0x3)
**Strategy ID:** 2 (0x2)
**Format:** `23|N|quotaName1|count1|startNano1|...|quotaNameN|countN|startNanoN`

### Data Structure
```go
type FixedWindow struct {
    Count int       // Request count in current window
    Start time.Time // Window start timestamp
}
```

### Format Breakdown
- `23`: Header (version 3, Fixed Window)
- `N`: Number of quotas (decimal)
- For each quota:
  - `quotaName`: String identifier
  - `count`: Current request count (decimal)
  - `startNano`: Window start time as Unix nanoseconds (int64)

### Example
```
23|2|default|3|1761884055342794596|hourly|10|1761884055342794596
```
Decoded:
- 2 quotas
- "default": 3 requests used, window started at 1761884055342794596 ns
- "hourly": 10 request used, same window start

### Key Characteristics
- Supports multi-quota configurations
- Start time: Unix nanoseconds (int64)
- TTL: Maximum reset time across all quotas

---

## 3. Leaky Bucket Strategy (Header: `32`)

**Version:** 2 (0x2)
**Strategy ID:** 3 (0x3)
**Format:** `32|requests|lastLeak_unix_nano`

### Data Structure
```go
type LeakyBucket struct {
    Requests float64   // Current requests in bucket
    LastLeak time.Time // Last leak timestamp
}
```

### Format Breakdown
- `32`: Header (version 2, Leaky Bucket)
- `requests`: Current requests in bucket (float64)
- `lastLeak`: Last leak time as Unix nanoseconds (int64)

### Example
```
32|5.0|1761884055342794596
```
Decoded:
- 5 requests currently queued
- Last leak at 1761884055342794596 ns

### Key Characteristics
- Requests as float64
- Single logical limit (no multiple quotas)
- Burst = max queue size

---

## 4. GCRA Strategy (Header: `42`)

**Version:** 2 (0x2)
**Strategy ID:** 4 (0x4)
**Format:** `42|tat_unix_nano`

### Data Structure
```go
type GCRA struct {
    TAT time.Time // Theoretical Arrival Time (next allowed request time)
}
```

### Format Breakdown
- `42`: Header (version 2, GCRA)
- `tat`: Theoretical Arrival Time as Unix nanoseconds (int64)

### Example
```
42|1761884055342794596
```
Decoded:
- Next allowed request at 1761884055342794596 ns

### Key Characteristics
- Simplest state (single timestamp)
- Single logical limit (no multiple quotas)
- TAT = next allowed request time

---

## 5. Composite Strategy (Header: `51`)

**Version:** 1 (0x1)
**Strategy ID:** 5 (0x5)
**Format:** `51|primaryState$secondaryState`

### Description
Atomic container for primary and secondary strategy states in dual-strategy configurations.

### Format Breakdown
- `51`: Header (version 1, Composite)
- `primaryState`: Encoded state of primary strategy
- `secondaryState`: Encoded state of secondary strategy
- `$`: Separator between primary and secondary states

### Example
```
51|23|1|default|2|1761884055342794596$12|8.5|1761884055342794596
```
Decoded:
- Primary: Fixed Window state (header `23`)
- Secondary: Token Bucket state (header `12`)

### Key Characteristics
- Contains nested strategy states
- Enables atomic Check-And-Set for both strategies
- Both strategies must approve requests
- Separated by `$` delimiter

---

## Internal Version History

Each strategy maintains its own independent internal version history for its data storage format. The version numbers track the evolution of each strategy's serialization format.

### Token Bucket Strategy (ID: 1)
- **Version 1** (`c55598d`): Initial custom ASCII format - `v1|tokens|lastrefill_ns|capacity|refill_rate`
- **Version 2** (`fddf090`): Optimized format - `v2|tokens|lastrefill_ns|` (removed runtime config)

### Fixed Window Strategy (ID: 2)
- **Version 1** (`c55598d`): Initial custom ASCII format - `v1|count|start_ns|duration_ns`
- **Version 2** (`fddf090`): Optimized format - `v2|count|start_ns|` (removed runtime config)
- **Version 3** (`88ecdd9`): Multi-quota combined format - `23|N|quotaName1|count1|startNano1|...|quotaNameN|countN|startNanoN`

### Leaky Bucket Strategy (ID: 3)
- **Version 1** (`c55598d`): Initial custom ASCII format - `v1|level|lastleak_ns|capacity|leak_rate`
- **Version 2** (`fddf090`): Optimized format - `v2|requests|lastleak_ns|` (removed runtime config)

### GCRA Strategy (ID: 4)
- **Version 1** (`ce0191a`): Initial custom ASCII format - `v1|tat_ns|emission_interval_ns|limit_ns`
- **Version 2** (`fddf090`): Optimized format - `v2|tat_ns|` (removed runtime config: emission_interval, limit)

### Composite Strategy (ID: 5)
- **Version 1** (`6d22bc7`): Initial composite format - `cmp1|<primaryState>$<secondaryState>` (atomic container for dual strategies)

### Key Transitions

#### `c55598d` - Performance Optimization (v1)
- Replaced JSON serialization with custom ASCII format
- Added version validation with `checkV1Header()`
- Improved performance and reduced storage overhead

#### `fddf090` - Runtime Config Removal (v2)
- Removed redundant fields from state storage
- Used configuration values instead of stored state values
- Simplified serialization format across all strategies

#### `88ecdd9` - Multi-Quota Support (Fixed Window v3)
- Implemented combined state approach for Fixed Window
- Added support for multiple quotas per key with atomic operations
- Replaced separate keys per quota with single combined state

#### `6d22bc7` - Composite Strategy Birth (v1)
- Introduced composite strategy for dual-strategy rate limiting
- Initial format: `cmp1|<primaryState>$<secondaryState>`
- Atomic container for primary and secondary strategy states
- Replaced separate primary/secondary keys with single composite key

#### `b467fa2` - Standardized Headers
- Replaced generic `"v2|"` and `"cmp1|"` headers with strategy-specific hex headers
- Implemented documented scheme: `{strategyID}{version}|`
- Composite changed from `"cmp1|"` to `"51|"`
- Added this DATA_FORMAT.md documentation

## References

- Fixed Window: `strategies/fixedwindow/internal/state.go`
- Token Bucket: `strategies/tokenbucket/internal/state.go`
- Leaky Bucket: `strategies/leakybucket/internal/state.go`
- GCRA: `strategies/gcra/internal/state.go`
- Composite: `internal/strategies/composite/state.go`
