-- 秒杀幂等检查脚本（Consumer 端使用，库存已在 API 层扣减）
-- KEYS[1]: 已购用户 Hash key (field=userID, value=已购数量)
-- KEYS[2]: 已处理用户 Hash key (field=userID, value=已处理数量)
-- ARGV[1]: 用户ID
-- ARGV[2]: 购买数量

-- 返回值:
-- 1: 成功，可以创建订单
-- -1: 用户未购买或数量不足（异常情况）
-- -2: 已处理过（重复消费，幂等返回）

local boughtKey = KEYS[1]
local processedKey = KEYS[2]
local userId = ARGV[1]
local quantity = tonumber(ARGV[2])

-- 1. 检查用户已购数量
local boughtCount = tonumber(redis.call('HGET', boughtKey, userId) or 0)
if boughtCount < quantity then
    return -1  -- 用户未购买或数量不足
end

-- 2. 检查已处理数量
local processedCount = tonumber(redis.call('HGET', processedKey, userId) or 0)
if processedCount >= boughtCount then
    return -2  -- 已全部处理过
end

-- 3. 增加已处理数量
redis.call('HINCRBY', processedKey, userId, quantity)
return 1
