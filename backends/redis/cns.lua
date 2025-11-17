--[[
Redis CheckAndSet Lua Script

This script implements atomic compare-and-swap semantics for rate limiting.

KEYS[1] - The Redis key to operate on (storage key)

ARGV[1] - Expected current value (oldValue)
        - Empty string "" for "set if not exists" semantics
        - Any other string for exact match comparison

ARGV[2] - New value to set (newValue)
          - The value to store if the comparison succeeds

ARGV[3] - Expiration time in milliseconds
          - "0" for no expiration (permanent key)
          - Any other number for TTL in milliseconds

Returns:
  1 - Operation succeeded (value was set)
  0 - Operation failed (comparison didn't match)
--]]

local current = redis.call('GET', KEYS[1])

-- If oldValue is empty, only set if key doesn't exist (set if not exists)
if ARGV[1] == '' then
  if current == false then
    if ARGV[3] == '0' then
      redis.call('SET', KEYS[1], ARGV[2])
    else
      redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
    end
    return 1
  end
  return 0
end

-- Check if current value matches oldValue (compare-and-swap)
if current == ARGV[1] then
  if ARGV[3] == '0' then
    redis.call('SET', KEYS[1], ARGV[2])
  else
    redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
  end
  return 1
end

return 0
