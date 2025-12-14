package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	warmupLockKey    = "seckill:warmup:lock:"
	warmupLockExpire = 30 * time.Second
)

// InitStock 初始化商品库存到 Redis - 分段存储
func InitStock(ctx context.Context, goodsID uint64, stock int) error {
	// 计算每个分段的库存
	baseStock := stock / SegmentCount
	remainder := stock % SegmentCount

	pipe := Client.Pipeline()
	for i := 0; i < SegmentCount; i++ {
		segmentStock := baseStock
		if i < remainder {
			segmentStock++ // 余数分配到前几个分段
		}
		segmentKey := SegmentStockKey(goodsID, i)
		pipe.Set(ctx, segmentKey, segmentStock, 0)
	}

	// 同时设置总库存（用于查询）
	stockKey := StockKey(goodsID)
	pipe.Set(ctx, stockKey, stock, 0)

	_, err := pipe.Exec(ctx)
	return err
}

// GetStock 获取当前总库存（汇总所有分段）
func GetStock(ctx context.Context, goodsID uint64) (int, error) {
	total := 0
	for i := 0; i < SegmentCount; i++ {
		segmentKey := SegmentStockKey(goodsID, i)
		val, err := Client.Get(ctx, segmentKey).Result()
		if err != nil {
			continue // 分段不存在则跳过
		}
		n, _ := strconv.Atoi(val)
		total += n
	}
	return total, nil
}

// ClearSeckillData 清理秒杀数据 (活动结束后调用)
func ClearSeckillData(ctx context.Context, goodsID uint64) error {
	keys := []string{StockKey(goodsID), BoughtKey(goodsID), DeductedKey(goodsID)}
	for i := 0; i < SegmentCount; i++ {
		keys = append(keys, SegmentStockKey(goodsID, i))
	}
	return Client.Del(ctx, keys...).Err()
}

// IncrSegmentStock 增加分段库存 (订单取消时返还)
func IncrSegmentStock(ctx context.Context, goodsID uint64, segmentID int) error {
	segmentKey := SegmentStockKey(goodsID, segmentID)
	return Client.Incr(ctx, segmentKey).Err()
}

// IncrSegmentStockBy 增加指定数量的分段库存（批量取消订单时返还）
func IncrSegmentStockBy(ctx context.Context, goodsID uint64, segmentID int, count int) error {
	segmentKey := SegmentStockKey(goodsID, segmentID)
	return Client.IncrBy(ctx, segmentKey, int64(count)).Err()
}

// IncrStock 增加库存到第一个分段 (用户主动取消订单时，没有分段信息)
func IncrStock(ctx context.Context, goodsID uint64) error {
	// 默认返还到分段0
	return IncrSegmentStock(ctx, goodsID, 0)
}

// ClearUserBought 清除用户购买记录 (订单取消时允许重新抢购)
func ClearUserBought(ctx context.Context, goodsID, userID uint64) error {
	boughtKey := BoughtKey(goodsID)
	return Client.HDel(ctx, boughtKey, fmt.Sprintf("%d", userID)).Err()
}

// AcquireWarmupLock 获取预热分布式锁
func AcquireWarmupLock(ctx context.Context, goodsID uint64) (bool, error) {
	lockKey := fmt.Sprintf("%s%d", warmupLockKey, goodsID)
	return Client.SetNX(ctx, lockKey, "1", warmupLockExpire).Result()
}

// ReleaseWarmupLock 释放预热分布式锁
func ReleaseWarmupLock(ctx context.Context, goodsID uint64) error {
	lockKey := fmt.Sprintf("%s%d", warmupLockKey, goodsID)
	return Client.Del(ctx, lockKey).Err()
}

// AcquireWarmupAllLock 获取全量预热分布式锁
func AcquireWarmupAllLock(ctx context.Context) (bool, error) {
	lockKey := warmupLockKey + "all"
	return Client.SetNX(ctx, lockKey, "1", warmupLockExpire).Result()
}

// ReleaseWarmupAllLock 释放全量预热分布式锁
func ReleaseWarmupAllLock(ctx context.Context) error {
	lockKey := warmupLockKey + "all"
	return Client.Del(ctx, lockKey).Err()
}
