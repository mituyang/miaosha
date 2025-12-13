package redis

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"

	"github.com/redis/go-redis/v9"
)

//go:embed seckill_segment.lua
var seckillSegmentScript string

//go:embed seckill_check.lua
var seckillCheckScript string

//go:embed seckill_decr.lua
var seckillDecrScript string

var seckillSegmentSHA string
var seckillCheckSHA string
var seckillDecrSHA string

// LoadScript 预加载 Lua 脚本到 Redis
func LoadScript(ctx context.Context) error {
	// 加载分段脚本
	sha, err := Client.ScriptLoad(ctx, seckillSegmentScript).Result()
	if err != nil {
		return err
	}
	seckillSegmentSHA = sha

	// 加载检查脚本
	sha2, err := Client.ScriptLoad(ctx, seckillCheckScript).Result()
	if err != nil {
		return err
	}
	seckillCheckSHA = sha2

	// 加载扣减脚本
	sha3, err := Client.ScriptLoad(ctx, seckillDecrScript).Result()
	if err != nil {
		return err
	}
	seckillDecrSHA = sha3

	return nil
}

// SeckillResult Lua 脚本返回值
type SeckillResult int

const (
	SeckillSuccess     SeckillResult = 1  // 成功
	SeckillSoldOut     SeckillResult = 0  // 库存不足
	SeckillRepeatBuy   SeckillResult = -1 // 重复购买
	SeckillNotStarted  SeckillResult = -2 // 活动未开始
	SeckillScriptError SeckillResult = -99
)

// PreDecrStock 预减库存 (原子操作) - 使用分段库存
// goodsID: 商品ID
// userID: 用户ID
// 返回: SeckillResult, 成功的分段索引（用于MySQL分段扣减）
func PreDecrStock(ctx context.Context, goodsID, userID uint64) (SeckillResult, int, error) {
	// 构建分段 keys
	keys := make([]string, SegmentCount+1)
	for i := 0; i < SegmentCount; i++ {
		keys[i] = SegmentStockKey(goodsID, i)
	}
	keys[SegmentCount] = BoughtKey(goodsID)

	// 随机起始分段，分散压力
	startIdx := rand.Intn(SegmentCount)

	result, err := Client.EvalSha(ctx, seckillSegmentSHA, keys, userID, SegmentCount, startIdx).Int()
	if err != nil {
		// 脚本不存在，重新加载
		if err == redis.Nil || err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			if loadErr := LoadScript(ctx); loadErr != nil {
				return SeckillScriptError, 0, loadErr
			}
			result, err = Client.EvalSha(ctx, seckillSegmentSHA, keys, userID, SegmentCount, startIdx).Int()
		}
		if err != nil {
			return SeckillScriptError, 0, err
		}
	}

	if result > 0 {
		// 返回成功，result 是分段索引（1-based），转为 0-based
		return SeckillSuccess, result - 1, nil
	}
	return SeckillResult(result), 0, nil
}

// RollbackStock 回滚库存 (下单失败时调用)
func RollbackStock(ctx context.Context, goodsID uint64, segmentID int, userID uint64) error {
	segmentKey := SegmentStockKey(goodsID, segmentID)
	boughtKey := BoughtKey(goodsID)
	deductedKey := DeductedKey(goodsID)
	userIDStr := fmt.Sprintf("%d", userID)

	pipe := Client.Pipeline()
	pipe.Incr(ctx, segmentKey)
	pipe.HDel(ctx, boughtKey, userIDStr)
	pipe.SRem(ctx, deductedKey, userID)
	_, err := pipe.Exec(ctx)
	return err
}

// CheckAndMark 检查用户资格、标记用户、扣减库存（原子操作）
// Redis 扣库存成功 = 秒杀成功
// 返回: SeckillResult, 扣减的分段索引
func CheckAndMark(ctx context.Context, goodsID, userID uint64) (SeckillResult, int, error) {
	// 构建分段 keys
	keys := make([]string, SegmentCount+1)
	for i := range SegmentCount {
		keys[i] = SegmentStockKey(goodsID, i)
	}
	keys[SegmentCount] = BoughtKey(goodsID)

	// 随机起始分段，分散压力
	startIdx := rand.Intn(SegmentCount)

	result, err := Client.EvalSha(ctx, seckillCheckSHA, keys, userID, SegmentCount, startIdx).Int()
	if err != nil {
		if err == redis.Nil || err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			if loadErr := LoadScript(ctx); loadErr != nil {
				return SeckillScriptError, 0, loadErr
			}
			result, err = Client.EvalSha(ctx, seckillCheckSHA, keys, userID, SegmentCount, startIdx).Int()
		}
		if err != nil {
			return SeckillScriptError, 0, err
		}
	}

	if result > 0 {
		return SeckillSuccess, result - 1, nil
	}
	return SeckillResult(result), 0, nil
}

// CheckProcessed Consumer 端幂等检查（库存已在 API 层扣减）
// 返回: 1=成功可创建订单, -1=用户未标记, -2=已处理过
func CheckProcessed(ctx context.Context, goodsID, userID uint64) (int, error) {
	boughtKey := BoughtKey(goodsID)
	processedKey := ProcessedKey(goodsID)

	result, err := Client.EvalSha(ctx, seckillDecrSHA, []string{boughtKey, processedKey}, userID).Int()
	if err != nil {
		if err == redis.Nil || err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			if loadErr := LoadScript(ctx); loadErr != nil {
				return -99, loadErr
			}
			result, err = Client.EvalSha(ctx, seckillDecrSHA, []string{boughtKey, processedKey}, userID).Int()
		}
		if err != nil {
			return -99, err
		}
	}
	return result, nil
}

// ClearUserMark 清除用户标记（MQ 发送失败时调用）
func ClearUserMark(ctx context.Context, goodsID, userID uint64) error {
	boughtKey := BoughtKey(goodsID)
	return Client.HDel(ctx, boughtKey, fmt.Sprintf("%d", userID)).Err()
}

// SetUserStatus 设置用户订单状态
// status: 0=待支付, 1=已支付, 2=已取消
func SetUserStatus(ctx context.Context, goodsID, userID uint64, status int) error {
	boughtKey := BoughtKey(goodsID)
	return Client.HSet(ctx, boughtKey, fmt.Sprintf("%d", userID), status).Err()
}

// ClearUserDeducted 清除用户已扣库存标记（废弃）
func ClearUserDeducted(ctx context.Context, goodsID, userID uint64) error {
	deductedKey := DeductedKey(goodsID)
	return Client.SRem(ctx, deductedKey, userID).Err()
}

// ClearProcessed 清除用户已处理标记（允许重试）
func ClearProcessed(ctx context.Context, goodsID, userID uint64) error {
	processedKey := ProcessedKey(goodsID)
	return Client.SRem(ctx, processedKey, userID).Err()
}
