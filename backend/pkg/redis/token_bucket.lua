-- 令牌桶限流算法
-- KEYS[1]: 令牌桶 key (ratelimit:user:{user_id})
-- ARGV[1]: 当前时间戳（秒）
-- ARGV[2]: 令牌生成速率（每秒）
-- ARGV[3]: 桶容量
-- ARGV[4]: key 过期时间（秒）
-- 返回: 1=通过, 0=限流

local key = KEYS[1]
local now = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local capacity = tonumber(ARGV[3])
local expire = tonumber(ARGV[4])

-- 获取当前令牌数和上次更新时间
local data = redis.call('HMGET', key, 'tokens', 'last_time')
local tokens = tonumber(data[1])
local last_time = tonumber(data[2])

-- 初始化：第一次访问
if tokens == nil or last_time == nil then
    tokens = capacity - 1  -- 扣除本次请求的令牌
    redis.call('HMSET', key, 'tokens', tokens, 'last_time', now)
    redis.call('EXPIRE', key, expire)
    return 1
end

-- 计算应该补充的令牌数
local elapsed = now - last_time
local add_tokens = elapsed * rate

-- 更新令牌数（不超过容量）
tokens = math.min(capacity, tokens + add_tokens)

-- 尝试扣除一个令牌
if tokens >= 1 then
    tokens = tokens - 1
    redis.call('HMSET', key, 'tokens', tokens, 'last_time', now)
    redis.call('EXPIRE', key, expire)
    return 1
else
    -- 令牌不足，更新时间但不扣除
    redis.call('HSET', key, 'last_time', now)
    redis.call('EXPIRE', key, expire)
    return 0
end
