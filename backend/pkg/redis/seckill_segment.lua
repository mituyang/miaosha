-- Redis 分段库存预减 Lua 脚本
-- KEYS[1..N]: 分段库存 keys (如 seckill:segment:1:0, seckill:segment:1:1, ...)
-- KEYS[N+1]: 已购用户集合 key (如 seckill:bought:1)
-- ARGV[1]: 用户ID
-- ARGV[2]: 分段数量
-- ARGV[3]: 起始分段索引（随机）

local segment_count = tonumber(ARGV[2])
local start_idx = tonumber(ARGV[3])
local user_id = ARGV[1]
local bought_key = KEYS[segment_count + 1]

-- 检查是否已购买
if redis.call('sismember', bought_key, user_id) == 1 then
    return -1  -- 已购买
end

-- 从随机位置开始轮询所有分段
for i = 0, segment_count - 1 do
    local idx = ((start_idx + i) % segment_count) + 1
    local segment_key = KEYS[idx]
    local stock = tonumber(redis.call('get', segment_key) or 0)
    
    if stock > 0 then
        -- 找到有库存的分段，扣减
        redis.call('decr', segment_key)
        redis.call('sadd', bought_key, user_id)
        return idx  -- 返回成功的分段索引（>0 表示成功）
    end
end

return 0  -- 所有分段都没库存
