-- 秒杀资格检查脚本（只检查+标记用户，不扣库存）
-- KEYS[1..N]: 分段库存 keys
-- KEYS[N+1]: 已购用户集合 key
-- ARGV[1]: 用户ID
-- ARGV[2]: 分段数量
-- ARGV[3]: 随机起始分段索引

-- 返回值:
-- > 0: 成功，返回有库存的分段索引（1-based）
-- 0: 库存不足
-- -1: 重复购买

local segmentCount = tonumber(ARGV[2])
local boughtKey = KEYS[segmentCount + 1]
local userId = ARGV[1]
local startIdx = tonumber(ARGV[3])

-- 1. 检查是否重复购买
if redis.call('SISMEMBER', boughtKey, userId) == 1 then
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

-- 3. 标记用户已购买（占位）
redis.call('SADD', boughtKey, userId)

-- 返回有库存的分段索引
return availableSegment
