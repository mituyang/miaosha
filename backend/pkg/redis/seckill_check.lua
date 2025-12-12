-- 秒杀资格检查脚本（只检查+标记用户，不扣库存）
-- KEYS[1..N]: 分段库存 keys
-- KEYS[N+1]: 已购用户 Hash key
-- ARGV[1]: 用户ID
-- ARGV[2]: 分段数量
-- ARGV[3]: 随机起始分段索引

-- 返回值:
-- > 0: 成功，返回有库存的分段索引（1-based）
-- 0: 库存不足
-- -1: 重复购买（状态为 0-待支付 或 1-已支付）

local segmentCount = tonumber(ARGV[2])
local boughtKey = KEYS[segmentCount + 1]
local userId = ARGV[1]
local startIdx = tonumber(ARGV[3])

-- 1. 检查是否重复购买（Hash: field=userID, value=状态）
-- 状态: 0=待支付, 1=已支付, 不存在=可抢购
local status = redis.call('HGET', boughtKey, userId)
if status and tonumber(status) ~= 2 then
    return -1
end

-- 2. 检查是否有库存（遍历所有分段）
local hasStock = false
local availableSegment = 0
for i = 0, segmentCount - 1 do
    local idx = (startIdx + i) % segmentCount
    local segmentKey = KEYS[idx + 1]
    local stock = tonumber(redis.call('GET', segmentKey) or 0)
    if stock > 0 then
        hasStock = true
        availableSegment = idx + 1  -- 1-based
        break
    end
end

if not hasStock then
    return 0
end

-- 3. 标记用户已购买（状态=0 待支付）
redis.call('HSET', boughtKey, userId, 0)

-- 返回有库存的分段索引
return availableSegment
