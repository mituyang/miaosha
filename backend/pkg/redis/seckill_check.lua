-- 秒杀核心脚本（检查 + 标记 + 扣库存，原子操作）
-- Redis 扣库存成功 = 秒杀成功，后续 MQ 只负责异步落库
-- KEYS[1]: 活动元数据 Hash key
-- KEYS[2..N+1]: 分段库存 keys
-- KEYS[N+2]: 已购用户 Hash key (field=userID, value=已购数量)
-- ARGV[1]: 用户ID
-- ARGV[2]: 分段数量
-- ARGV[3]: 随机起始分段索引
-- ARGV[4]: 购买数量
-- ARGV[5]: 请求时间戳(毫秒)

-- 返回值:
-- > 0: 成功，返回扣减的分段索引（1-based）
-- 0: 库存不足
-- -1: 超过限购数量
-- -2: 活动不可抢购

local segmentCount = tonumber(ARGV[2])
local metaKey = KEYS[1]
local boughtKey = KEYS[segmentCount + 2]
local userId = ARGV[1]
local startIdx = tonumber(ARGV[3])
local quantity = tonumber(ARGV[4])
local requestTime = tonumber(ARGV[5])

local status = tonumber(redis.call('HGET', metaKey, 'status') or -1)
local startTime = tonumber(redis.call('HGET', metaKey, 'start_time') or 0)
local endTime = tonumber(redis.call('HGET', metaKey, 'end_time') or 0)
local maxBuyLimit = tonumber(redis.call('HGET', metaKey, 'max_buy_limit') or 0)
local warmupStatus = tonumber(redis.call('HGET', metaKey, 'warmup_status') or 0)
local goodsOnSale = tonumber(redis.call('HGET', metaKey, 'goods_on_sale') or 0)

if goodsOnSale ~= 1 or warmupStatus ~= 1 or maxBuyLimit <= 0 then
    return -2
end

if status ~= 0 and status ~= 1 then
    return -2
end

if requestTime < startTime or requestTime > endTime then
    return -2
end

-- 1. 检查已购数量
local currentBought = tonumber(redis.call('HGET', boughtKey, userId) or 0)
if currentBought + quantity > maxBuyLimit then
    return -1  -- 超过限购数量
end

-- 2. 遍历分段，找到有足够库存的分段并扣减
for i = 0, segmentCount - 1 do
    local idx = (startIdx + i) % segmentCount
    local segmentKey = KEYS[idx + 2]
    local stock = tonumber(redis.call('GET', segmentKey) or 0)
    if stock >= quantity then
        -- 3. 扣减库存
        redis.call('DECRBY', segmentKey, quantity)
        -- 4. 增加用户已购数量
        redis.call('HINCRBY', boughtKey, userId, quantity)
        -- 返回扣减的分段索引（1-based）
        return idx + 1
    end
end

-- 所有分段都没有足够库存
return 0
