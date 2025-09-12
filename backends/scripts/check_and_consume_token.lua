-- check_and_consume_token.lua
-- Atomically checks if a token is available and consumes it
-- Used for Token Bucket rate limiting strategy
--
-- KEYS[1]: rate limit key
-- ARGV[1]: bucket_size (maximum number of tokens in bucket)
-- ARGV[2]: refill_rate_seconds (how often to refill tokens, in seconds)
-- ARGV[3]: refill_amount (number of tokens to add per refill)
-- ARGV[4]: window_seconds (duration for TTL calculation)
-- ARGV[5]: now (current timestamp in seconds)
--
-- Returns: {tokens, consumed, requests} where:
--   tokens: current number of tokens after operation
--   consumed: 1 if token consumed, 0 if not (insufficient tokens)
--   requests: total number of requests processed

local key = KEYS[1]
local bucket_size = tonumber(ARGV[1])
local refill_rate_seconds = tonumber(ARGV[2])
local refill_amount = tonumber(ARGV[3])
local window_seconds = tonumber(ARGV[4])
local now = tonumber(ARGV[5])

-- Get current data
local current = redis.call('GET', key)
local data = {}

if current then
    data = cjson.decode(current)
else
    -- First request - start with full bucket minus one token
    if bucket_size > 0 then
        data = {
            count = 1,
            window_start = now - window_seconds,
            last_request = now,
            tokens = bucket_size - 1.0,
            last_refill = now,
            metadata = {}
        }
        
        -- Store updated data with TTL
        local ttl = window_seconds * 2
        redis.call('SET', key, cjson.encode(data), 'EX', ttl)
        
        return {data.tokens, 1, data.count}  -- tokens, consumed=true, requests
    else
        return {0, 0, 0}  -- tokens=0, consumed=false, requests=0
    end
end

-- Calculate tokens to add based on time elapsed
local elapsed = now - data.last_refill
local refill_periods = elapsed / refill_rate_seconds

if refill_periods > 0 then
    local tokens_to_add = refill_periods * refill_amount
    data.tokens = math.min(data.tokens + tokens_to_add, bucket_size)
    -- Advance last_refill by the duration of the periods we've just accounted for.
    -- This preserves any fractional time for the next calculation.
    data.last_refill = data.last_refill + (refill_periods * refill_rate_seconds)
end

-- Check if we can consume a token
if data.tokens >= 1.0 then
    -- Consume one token
    data.tokens = data.tokens - 1.0
    data.count = data.count + 1
    data.last_request = now
    data.window_start = now - window_seconds  -- Rolling window
    
    -- Store updated data with TTL
    local ttl = window_seconds * 2
    redis.call('SET', key, cjson.encode(data), 'EX', ttl)
    
    return {data.tokens, 1, data.count}  -- tokens, consumed=true, requests
else
    -- Not enough tokens - update last_request but don't consume
    data.last_request = now
    
    -- Store updated data with TTL
    local ttl = window_seconds * 2
    redis.call('SET', key, cjson.encode(data), 'EX', ttl)
    
    return {data.tokens, 0, data.count}  -- tokens, consumed=false, requests
end