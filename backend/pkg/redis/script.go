package redis

import (
	"context"
	_ "embed"

	"github.com/redis/go-redis/v9"
)

//go:embed seckill.lua
var seckillScript string

var seckillSHA string

// LoadScript 预加载 Lua 脚本到 Redis
func LoadScript(ctx context.Context) error {
	sha, err := Client.ScriptLoad(ctx, seckillScript).Result()
	if err != nil {
		return err
	}
	seckillSHA = sha
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

// PreDecrStock 预减库存 (原子操作)
// goodsID: 商品ID
// userID: 用户ID
// 返回: SeckillResult
func PreDecrStock(ctx context.Context, goodsID, userID uint64) (SeckillResult, error) {
	stockKey := StockKey(goodsID)
	boughtKey := BoughtKey(goodsID)

	result, err := Client.EvalSha(ctx, seckillSHA, []string{stockKey, boughtKey}, userID).Int()
	if err != nil {
		// 脚本不存在，重新加载
		if err == redis.Nil || err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			if loadErr := LoadScript(ctx); loadErr != nil {
				return SeckillScriptError, loadErr
			}
			result, err = Client.EvalSha(ctx, seckillSHA, []string{stockKey, boughtKey}, userID).Int()
		}
		if err != nil {
			return SeckillScriptError, err
		}
	}

	return SeckillResult(result), nil
}

// RollbackStock 回滚库存 (下单失败时调用)
func RollbackStock(ctx context.Context, goodsID, userID uint64) error {
	stockKey := StockKey(goodsID)
	boughtKey := BoughtKey(goodsID)

	pipe := Client.Pipeline()
	pipe.Incr(ctx, stockKey)
	pipe.SRem(ctx, boughtKey, userID)
	_, err := pipe.Exec(ctx)
	return err
}
