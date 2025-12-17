-- Redis 延迟队列原子弹出 Lua 脚本
-- KEYS[1]: 超时队列 key (seckill:order:timeout)
-- ARGV[1]: 当前时间戳（秒）
-- ARGV[2]: 最大返回数量

local key = KEYS[1]
local now = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])

-- 获取已过期的订单（score <= now）
local orders = redis.call('ZRANGEBYSCORE', key, '0', now, 'LIMIT', 0, limit)

if #orders > 0 then
    -- 原子删除这些订单
    redis.call('ZREM', key, unpack(orders))
end

return orders
