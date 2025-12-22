package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed delay_queue_pop.lua
var delayQueuePopScript string

var delayQueuePopSHA string

const (
	orderTimeoutZSetKey = "seckill:order:timeout"      // ZSET: score=过期时间, member=orderID
	orderTimeoutHashKey = "seckill:order:timeout:data" // Hash: field=orderID, value=JSON
)

// OrderTimeoutItem 订单超时队列项
type OrderTimeoutItem struct {
	OrderID   uint64 `json:"order_id"`
	UserID    uint64 `json:"user_id"`
	GoodsID   uint64 `json:"goods_id"`
	SegmentID int    `json:"segment_id"`
	Quantity  int    `json:"quantity"` // 购买数量
}

// LoadDelayQueueScript 加载延迟队列 Lua 脚本
func LoadDelayQueueScript(ctx context.Context) error {
	sha, err := Client.ScriptLoad(ctx, delayQueuePopScript).Result()
	if err != nil {
		return err
	}
	delayQueuePopSHA = sha
	return nil
}

// AddOrderTimeout 添加订单到超时队列
// expireAt: 订单过期时间
func AddOrderTimeout(ctx context.Context, orderID, userID, goodsID uint64, segmentID int, quantity int, expireAt time.Time) error {
	item := OrderTimeoutItem{
		OrderID:   orderID,
		UserID:    userID,
		GoodsID:   goodsID,
		SegmentID: segmentID,
		Quantity:  quantity,
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	orderIDStr := strconv.FormatUint(orderID, 10)

	// Pipeline: ZADD + HSET
	pipe := Client.Pipeline()
	pipe.ZAdd(ctx, orderTimeoutZSetKey, redis.Z{
		Score:  float64(expireAt.Unix()),
		Member: orderIDStr,
	})
	pipe.HSet(ctx, orderTimeoutHashKey, orderIDStr, string(data))
	_, err = pipe.Exec(ctx)
	return err
}

// PopExpiredOrders 获取并移除已过期的订单（原子操作）
// 返回过期的订单列表
func PopExpiredOrders(ctx context.Context, limit int64) ([]OrderTimeoutItem, error) {
	now := float64(time.Now().Unix())

	// 使用 Lua 脚本原子获取并删除过期的 orderID
	result, err := Client.EvalSha(ctx, delayQueuePopSHA, []string{orderTimeoutZSetKey}, now, limit).StringSlice()
	if err != nil {
		if err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			if loadErr := LoadDelayQueueScript(ctx); loadErr != nil {
				return nil, loadErr
			}
			result, err = Client.EvalSha(ctx, delayQueuePopSHA, []string{orderTimeoutZSetKey}, now, limit).StringSlice()
		}
		if err != nil {
			return nil, err
		}
	}

	if len(result) == 0 {
		return nil, nil
	}

	// 从 Hash 获取订单详情
	items := make([]OrderTimeoutItem, 0, len(result))
	for _, orderIDStr := range result {
		data, err := Client.HGet(ctx, orderTimeoutHashKey, orderIDStr).Result()
		if err != nil {
			continue
		}

		var item OrderTimeoutItem
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			continue
		}
		items = append(items, item)

		// 删除 Hash 中的数据
		_ = Client.HDel(ctx, orderTimeoutHashKey, orderIDStr)
	}

	return items, nil
}

// RemoveOrderTimeout 从超时队列移除订单（支付成功时调用）
func RemoveOrderTimeout(ctx context.Context, orderID uint64) error {
	orderIDStr := strconv.FormatUint(orderID, 10)

	// Pipeline: ZREM + HDEL
	pipe := Client.Pipeline()
	pipe.ZRem(ctx, orderTimeoutZSetKey, orderIDStr)
	pipe.HDel(ctx, orderTimeoutHashKey, orderIDStr)
	_, err := pipe.Exec(ctx)
	return err
}

// GetTimeoutQueueSize 获取超时队列大小（用于监控）
func GetTimeoutQueueSize(ctx context.Context) (int64, error) {
	return Client.ZCard(ctx, orderTimeoutZSetKey).Result()
}

// GetPendingTimeoutCount 获取待处理的超时订单数量
func GetPendingTimeoutCount(ctx context.Context) (int64, error) {
	now := fmt.Sprintf("%d", time.Now().Unix())
	return Client.ZCount(ctx, orderTimeoutZSetKey, "0", now).Result()
}

// AddOrderTimeoutBatch 批量添加订单到超时队列
func AddOrderTimeoutBatch(ctx context.Context, items []OrderTimeoutItem, expireAt time.Time) error {
	if len(items) == 0 {
		return nil
	}

	pipe := Client.Pipeline()
	score := float64(expireAt.Unix())

	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			continue
		}
		orderIDStr := strconv.FormatUint(item.OrderID, 10)

		pipe.ZAdd(ctx, orderTimeoutZSetKey, redis.Z{
			Score:  score,
			Member: orderIDStr,
		})
		pipe.HSet(ctx, orderTimeoutHashKey, orderIDStr, string(data))
	}

	_, err := pipe.Exec(ctx)
	return err
}
