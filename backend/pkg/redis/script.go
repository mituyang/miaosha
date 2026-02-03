package redis

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"strings"

	redisLib "github.com/redis/go-redis/v9"
)

//go:embed seckill_segment.lua
var seckillSegmentScript string

//go:embed seckill_check.lua
var seckillCheckScript string

//go:embed seckill_decr.lua
var seckillDecrScript string

//go:embed token_bucket.lua
var tokenBucketScript string

var seckillSegmentSHA string
var seckillCheckSHA string
var seckillDecrSHA string
var tokenBucketSHA string

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

	// 加载令牌桶脚本
	sha4, err := Client.ScriptLoad(ctx, tokenBucketScript).Result()
	if err != nil {
		return err
	}
	tokenBucketSHA = sha4

	return nil
}

// isNoScriptError 判断是否是脚本不存在错误
func isNoScriptError(err error) bool {
	if err == nil {
		return false
	}
	if err == redisLib.Nil {
		return true
	}
	return strings.Contains(err.Error(), "NOSCRIPT")
}

// evalShaWithRetry 执行Lua脚本，如果脚本不存在则重新加载后重试
func evalShaWithRetry(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error) {
	result, err := Client.EvalSha(ctx, sha, keys, args...).Result()
	if err != nil && isNoScriptError(err) {
		if loadErr := LoadScript(ctx); loadErr != nil {
			return nil, loadErr
		}
		return Client.EvalSha(ctx, sha, keys, args...).Result()
	}
	return result, err
}

// evalShaIntWithRetry 执行Lua脚本并返回int结果
func evalShaIntWithRetry(ctx context.Context, sha string, keys []string, args ...interface{}) (int, error) {
	result, err := evalShaWithRetry(ctx, sha, keys, args...)
	if err != nil {
		return 0, err
	}
	switch v := result.(type) {
	case int64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected result type: %T", result)
	}
}

// SeckillResult Lua 脚本返回值
type SeckillResult int

const (
	SeckillSuccess     SeckillResult = 1  // 成功
	SeckillSoldOut     SeckillResult = 0  // 库存不足
	SeckillLimitExceed SeckillResult = -1 // 超过限购数量
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

	result, err := evalShaIntWithRetry(ctx, seckillSegmentSHA, keys, userID, SegmentCount, startIdx)
	if err != nil {
		return SeckillScriptError, 0, err
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
func CheckAndMark(ctx context.Context, goodsID, userID uint64, quantity, maxBuyLimit int) (SeckillResult, int, error) {
	// 构建分段 keys
	keys := make([]string, SegmentCount+1)
	for i := range SegmentCount {
		keys[i] = SegmentStockKey(goodsID, i)
	}
	keys[SegmentCount] = BoughtKey(goodsID)

	// 随机起始分段，分散压力
	startIdx := rand.Intn(SegmentCount)

	result, err := evalShaIntWithRetry(ctx, seckillCheckSHA, keys, userID, SegmentCount, startIdx, quantity, maxBuyLimit)
	if err != nil {
		return SeckillScriptError, 0, err
	}

	if result > 0 {
		return SeckillSuccess, result - 1, nil
	}
	return SeckillResult(result), 0, nil
}

// CheckProcessed Consumer 端幂等检查（库存已在 API 层扣减）
// 返回: 1=成功可创建订单, -1=用户未标记, -2=已处理过
func CheckProcessed(ctx context.Context, goodsID, userID uint64, quantity int) (int, error) {
	boughtKey := BoughtKey(goodsID)
	processedKey := ProcessedKey(goodsID)

	result, err := evalShaIntWithRetry(ctx, seckillDecrSHA, []string{boughtKey, processedKey}, userID, quantity)
	if err != nil {
		return -99, err
	}
	return result, nil
}

// ClearUserMark 清除用户标记（MQ 发送失败时调用，减少已购数量）
func ClearUserMark(ctx context.Context, goodsID, userID uint64, quantity int) error {
	boughtKey := BoughtKey(goodsID)
	field := fmt.Sprintf("%d", userID)
	val, err := Client.HIncrBy(ctx, boughtKey, field, int64(-quantity)).Result()
	if err != nil {
		return err
	}
	// 如果值 <= 0，删除该 field
	if val <= 0 {
		_ = Client.HDel(ctx, boughtKey, field).Err()
	}
	return nil
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

// ClearProcessed 清除用户已处理标记（允许重试，减少已处理数量）
func ClearProcessed(ctx context.Context, goodsID, userID uint64, quantity int) error {
	processedKey := ProcessedKey(goodsID)
	field := fmt.Sprintf("%d", userID)
	val, err := Client.HIncrBy(ctx, processedKey, field, int64(-quantity)).Result()
	if err != nil {
		return err
	}
	// 如果值 <= 0，删除该 field
	if val <= 0 {
		_ = Client.HDel(ctx, processedKey, field).Err()
	}
	return nil
}

// BatchCheckItem 批量检查项
type BatchCheckItem struct {
	UserID   uint64
	Quantity int
}

// CheckProcessedBatch 批量幂等检查（使用 Pipeline）
// 返回: map[userID]result, 1=成功, -1=未标记, -2=已处理
func CheckProcessedBatch(ctx context.Context, goodsID uint64, items []BatchCheckItem) (map[uint64]int, error) {
	if len(items) == 0 {
		return make(map[uint64]int), nil
	}

	boughtKey := BoughtKey(goodsID)
	processedKey := ProcessedKey(goodsID)

	// 执行批量脚本调用
	execBatch := func() ([]*redisLib.Cmd, error) {
		pipe := Client.Pipeline()
		cmds := make([]*redisLib.Cmd, len(items))
		for i, item := range items {
			cmds[i] = pipe.EvalSha(ctx, seckillDecrSHA, []string{boughtKey, processedKey}, item.UserID, item.Quantity)
		}
		_, err := pipe.Exec(ctx)
		return cmds, err
	}

	cmds, err := execBatch()
	if err != nil && err != redisLib.Nil && isNoScriptError(err) {
		// 脚本不存在，重新加载后重试
		if loadErr := LoadScript(ctx); loadErr != nil {
			return nil, loadErr
		}
		cmds, err = execBatch()
		if err != nil && err != redisLib.Nil {
			return nil, err
		}
	}

	results := make(map[uint64]int, len(items))
	for i, cmd := range cmds {
		result, err := cmd.Int()
		if err != nil {
			results[items[i].UserID] = -99
		} else {
			results[items[i].UserID] = result
		}
	}

	return results, nil
}

// BatchRestoreStock 批量返还库存和清除用户标记
func BatchRestoreStock(ctx context.Context, items []OrderTimeoutItem) error {
	if len(items) == 0 {
		return nil
	}

	pipe := Client.Pipeline()

	// 记录需要检查的字段
	type fieldInfo struct {
		key   string
		field string
	}
	var boughtFields []fieldInfo
	var processedFields []fieldInfo

	for _, item := range items {
		quantity := item.Quantity
		if quantity <= 0 {
			quantity = 1
		}

		// 返还分段库存
		segmentKey := SegmentStockKey(item.GoodsID, item.SegmentID)
		pipe.IncrBy(ctx, segmentKey, int64(quantity))

		// 减少用户已购数量
		boughtKey := BoughtKey(item.GoodsID)
		field := fmt.Sprintf("%d", item.UserID)
		pipe.HIncrBy(ctx, boughtKey, field, int64(-quantity))
		boughtFields = append(boughtFields, fieldInfo{key: boughtKey, field: field})

		// 减少已处理数量
		processedKey := ProcessedKey(item.GoodsID)
		pipe.HIncrBy(ctx, processedKey, field, int64(-quantity))
		processedFields = append(processedFields, fieldInfo{key: processedKey, field: field})
	}

	results, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	// 检查 HIncrBy 结果，删除值为 0 或以下的字段
	// 结果顺序: [segmentIncrBy, boughtHIncrBy, processedHIncrBy] * len(items)
	delPipe := Client.Pipeline()
	hasDeletes := false

	for i := range items {
		// boughtHIncrBy 结果在位置 i*3+1
		boughtIdx := i*3 + 1
		if boughtIdx < len(results) {
			if cmd, ok := results[boughtIdx].(*redisLib.IntCmd); ok {
				if val, _ := cmd.Result(); val <= 0 {
					delPipe.HDel(ctx, boughtFields[i].key, boughtFields[i].field)
					hasDeletes = true
				}
			}
		}

		// processedHIncrBy 结果在位置 i*3+2
		processedIdx := i*3 + 2
		if processedIdx < len(results) {
			if cmd, ok := results[processedIdx].(*redisLib.IntCmd); ok {
				if val, _ := cmd.Result(); val <= 0 {
					delPipe.HDel(ctx, processedFields[i].key, processedFields[i].field)
					hasDeletes = true
				}
			}
		}
	}

	if hasDeletes {
		_, _ = delPipe.Exec(ctx)
	}

	return nil
}
