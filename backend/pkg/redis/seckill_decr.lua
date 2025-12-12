-- 秒杀幂等检查脚本（Consumer 端使用，库存已在 API 层扣减）
-- KEYS[1]: 已购用户 Hash key (标记)
-- KEYS[2]: 已处理用户集合 key (防重复消费)
-- ARGV[1]: 用户ID

-- 返回值:
-- 1: 成功，可以创建订单
-- -1: 用户未标记或已取消（异常情况，不应该收到这个消息）
-- -2: 已处理过（重复消费，幂等返回）

local boughtKey = KEYS[1]
local processedKey = KEYS[2]
local userId = ARGV[1]

-- 1. 检查用户是否已标记且状态为待支付
local status = redis.call('HGET', boughtKey, userId)
if not status or tonumber(status) ~= 0 then
    return -1
end

-- 2. 检查是否已处理过（防止重复创建订单）
if redis.call('SISMEMBER', processedKey, userId) == 1 then
    return -2
end

-- 3. 标记已处理
redis.call('SADD', processedKey, userId)
return 1
