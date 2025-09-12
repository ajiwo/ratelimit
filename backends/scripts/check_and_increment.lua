-- check_and_increment.lua
-- Atomically checks if incrementing would exceed the limit and increments if allowed
-- Used for Fixed Window rate limiting strategy
--
-- KEYS[1]: rate limit key
-- ARGV[1]: window_seconds (duration of the window in seconds)
-- ARGV[2]: now (current timestamp in seconds)
-- ARGV[3]: limit (maximum requests allowed in the window)
--
-- Returns: {count, incremented} where:
--   count: current request count after operation
--   incremented: 1 if incremented, 0 if not (due to limit)

local key = KEYS[1]
local window_seconds = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

-- Get current data
local current = redis.call('GET', key)
local data = {}

if current then
    data = cjson.decode(current)
else
    data = {
        count = 0,
        window_start = now,
        last_request = now,
        tokens = 0,
        last_refill = now,
        metadata = {}
    }
end

-- Check if window has expired
if (now - data.window_start) >= window_seconds then
    -- Reset window - first request in new window
    if limit > 0 then
        data.count = 1
        data.window_start = now
        data.last_request = now
        
        -- Store updated data with TTL
        local ttl = window_seconds * 2
        redis.call('SET', key, cjson.encode(data), 'EX', ttl)
        
        return {data.count, 1}  -- count, incremented=true
    else
        return {0, 0}  -- count=0, incremented=false
    end
else
    -- Check if incrementing would exceed limit
    if data.count >= limit then
        -- Would exceed limit - don't increment
        return {data.count, 0}  -- count, incremented=false
    else
        -- Increment within current window
        data.count = data.count + 1
        data.last_request = now
        
        -- Store updated data with TTL
        local ttl = window_seconds * 2
        redis.call('SET', key, cjson.encode(data), 'EX', ttl)
        
        return {data.count, 1}  -- count, incremented=true
    end
end