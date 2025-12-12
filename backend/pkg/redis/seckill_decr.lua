-- 秒杀库存扣减脚本（Consumer 端使用）
-- KEYS[1]: 分段库存 key
-- KEYS[2]: 已购用户 Hash key (标记)
-- KEYS[3]: 已扣库存用户集合 key (防重复扣减)
-- ARGV[1]: 用户ID

-- 返回值:
-- 1: 成功
-- 0: 库存不足
-- -1: 用户未标记或已取消（异常情况）
-- -2: 已扣过库存（重复消费）

local segmentKey = KEYS[1]
local boughtKey = KEYS[2]
local deductedKey = KEYS[3]
local userId = ARGV[1]

-- 1. 检查用户是否已标记且状态为待支付
local status = redis.call('HGET', boughtKey, userId)
if not status or tonumber(status) ~= 0 then
    return -1
end

-- 2. 检查是否已扣过库存（防止重复扣减）
if redis.call('SISMEMBER', deductedKey, userId) == 1 then
    return -2
end

-- 3. 扣减库存
local stock = tonumber(redis.call('GET', segmentKey) or 0)
if stock <= 0 then
    return 0
end

redis.call('DECR', segmentKey)
-- 4. 标记已扣库存
redis.call('SADD', deductedKey, userId)
return 1
