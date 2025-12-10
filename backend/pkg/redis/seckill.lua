-- Redis 预减库存 Lua 脚本
-- KEYS[1]: 库存 key (如 seckill:stock:1)
-- KEYS[2]: 已购用户集合 key (如 seckill:bought:1)
-- ARGV[1]: 用户ID

local stock_key = KEYS[1]
local bought_key = KEYS[2]
local user_id = ARGV[1]

-- 检查是否已购买
if redis.call('sismember', bought_key, user_id) == 1 then
    return -1  -- 已购买
end

-- 检查库存
local stock = tonumber(redis.call('get', stock_key) or 0)
if stock <= 0 then
    return 0  -- 库存不足
end

-- 扣减库存 & 记录已购
redis.call('decr', stock_key)
redis.call('sadd', bought_key, user_id)

return 1  -- 成功
