package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

var Client *redis.Client

// InitRedis 初始化 Redis 连接
func InitRedis(addr, password string, db int) error {
	Client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		PoolSize: 100, // 连接池大小，高并发场景需要调大
	})

	ctx := context.Background()
	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis 连接失败: %w", err)
	}
	return nil
}

// SeckillScript 秒杀 Lua 脚本 - 原子性扣减库存并防止重复秒杀
// KEYS[1]: 库存 key (seckill:stock:{goods_id})
// KEYS[2]: 已购用户集合 key (seckill:bought:{goods_id})
// ARGV[1]: user_id
// 返回值: 1-成功, 0-库存不足, -1-重复秒杀
var SeckillScript = redis.NewScript(`
	-- 检查是否已经秒杀过
	if redis.call('sismember', KEYS[2], ARGV[1]) == 1 then
		return -1
	end
	-- 检查库存
	local stock = tonumber(redis.call('get', KEYS[1]))
	if stock == nil or stock <= 0 then
		return 0
	end
	-- 扣减库存
	redis.call('decr', KEYS[1])
	-- 记录已购用户
	redis.call('sadd', KEYS[2], ARGV[1])
	return 1
`)

// RollbackScript 回滚 Lua 脚本 - 原子性恢复库存并移除用户记录
// 用于 Kafka 发送失败时的补偿操作
// KEYS[1]: 库存 key
// KEYS[2]: 已购用户集合 key
// ARGV[1]: user_id
var RollbackScript = redis.NewScript(`
	-- 恢复库存
	redis.call('incr', KEYS[1])
	-- 移除已购记录
	redis.call('srem', KEYS[2], ARGV[1])
	return 1
`)

// DoSeckill 执行秒杀 Lua 脚本
// userID 为用户名（字符串）
func DoSeckill(ctx context.Context, goodsID int64, userID string) (int64, error) {
	stockKey := fmt.Sprintf("seckill:stock:%d", goodsID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", goodsID)

	result, err := SeckillScript.Run(ctx, Client, []string{stockKey, boughtKey}, userID).Int64()
	if err != nil {
		return 0, fmt.Errorf("执行秒杀脚本失败: %w", err)
	}
	return result, nil
}

// RollbackSeckill 回滚秒杀操作（Kafka 发送失败时调用）
// userID 为用户名（字符串）
func RollbackSeckill(ctx context.Context, goodsID int64, userID string) error {
	stockKey := fmt.Sprintf("seckill:stock:%d", goodsID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", goodsID)

	_, err := RollbackScript.Run(ctx, Client, []string{stockKey, boughtKey}, userID).Int64()
	if err != nil {
		return fmt.Errorf("回滚秒杀失败: %w", err)
	}
	return nil
}

// PreloadStock 预热库存到 Redis
func PreloadStock(ctx context.Context, goodsID int64, stock int) error {
	stockKey := fmt.Sprintf("seckill:stock:%d", goodsID)
	return Client.Set(ctx, stockKey, stock, 0).Err()
}

// 延迟队列相关常量
const (
	OrderDelayQueueKey = "order:delay:queue" // 订单延迟队列 key
	OrderTimeout       = 60                  // 订单超时时间（秒）
)

// AddToDelayQueue 将订单添加到延迟队列
// score 为订单过期时间戳（当前时间 + 60秒）
func AddToDelayQueue(ctx context.Context, orderID string) error {
	expireAt := float64(time.Now().Unix() + OrderTimeout)
	return Client.ZAdd(ctx, OrderDelayQueueKey, &redis.Z{
		Score:  expireAt,
		Member: orderID,
	}).Err()
}

// GetExpiredOrders 获取已过期的订单（score <= 当前时间戳）
// 返回订单ID列表，最多返回 limit 条
func GetExpiredOrders(ctx context.Context, limit int64) ([]string, error) {
	now := float64(time.Now().Unix())
	// ZRANGEBYSCORE order:delay:queue 0 <now> LIMIT 0 <limit>
	return Client.ZRangeByScore(ctx, OrderDelayQueueKey, &redis.ZRangeBy{
		Min:   "0",
		Max:   fmt.Sprintf("%f", now),
		Count: limit,
	}).Result()
}

// RemoveFromDelayQueue 从延迟队列中移除订单
func RemoveFromDelayQueue(ctx context.Context, orderID string) error {
	return Client.ZRem(ctx, OrderDelayQueueKey, orderID).Err()
}

// ==================== 订单缓存相关 ====================

// OrderCache 订单缓存结构
type OrderCache struct {
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	GoodsID   int64  `json:"goods_id"`
	Status    int8   `json:"status"`     // 0-待支付, 1-已支付, 2-已取消
	CreatedAt int64  `json:"created_at"` // 毫秒时间戳
}

// 订单缓存过期时间（2小时，足够覆盖支付超时）
const OrderCacheExpire = 2 * time.Hour

// orderCacheKey 生成订单缓存 key
func orderCacheKey(orderID string) string {
	return fmt.Sprintf("order:cache:%s", orderID)
}

// userOrdersKey 生成用户订单列表 key
func userOrdersKey(userID string) string {
	return fmt.Sprintf("user:orders:%s", userID)
}

// SetOrderCache 缓存订单到 Redis
func SetOrderCache(ctx context.Context, order *OrderCache) error {
	key := orderCacheKey(order.OrderID)
	data := fmt.Sprintf("%s|%d|%d|%d", order.UserID, order.GoodsID, order.Status, order.CreatedAt)

	pipe := Client.Pipeline()
	// 存储订单详情
	pipe.Set(ctx, key, data, OrderCacheExpire)
	// 添加到用户订单列表（用于查询我的订单）
	pipe.SAdd(ctx, userOrdersKey(order.UserID), order.OrderID)
	pipe.Expire(ctx, userOrdersKey(order.UserID), OrderCacheExpire)

	_, err := pipe.Exec(ctx)
	return err
}

// GetOrderCache 从 Redis 获取订单缓存
func GetOrderCache(ctx context.Context, orderID string) (*OrderCache, error) {
	key := orderCacheKey(orderID)
	data, err := Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // 缓存不存在
	}
	if err != nil {
		return nil, err
	}

	// 解析数据：userID|goodsID|status|createdAt
	parts := strings.Split(data, "|")
	if len(parts) != 4 {
		return nil, fmt.Errorf("订单缓存格式错误: %s", data)
	}

	goodsID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("解析 goodsID 失败: %w", err)
	}
	status, err := strconv.ParseInt(parts[2], 10, 8)
	if err != nil {
		return nil, fmt.Errorf("解析 status 失败: %w", err)
	}
	createdAt, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("解析 createdAt 失败: %w", err)
	}

	return &OrderCache{
		OrderID:   orderID,
		UserID:    parts[0],
		GoodsID:   goodsID,
		Status:    int8(status),
		CreatedAt: createdAt,
	}, nil
}

// UpdateOrderStatus 更新订单状态（Redis 缓存）
func UpdateOrderStatus(ctx context.Context, orderID string, status int8) error {
	order, err := GetOrderCache(ctx, orderID)
	if err != nil || order == nil {
		return err // 缓存不存在，不更新
	}
	order.Status = status
	return SetOrderCache(ctx, order)
}

// GetUserOrderIDs 获取用户的订单ID列表
func GetUserOrderIDs(ctx context.Context, userID string) ([]string, error) {
	return Client.SMembers(ctx, userOrdersKey(userID)).Result()
}

// DeleteOrderCache 删除订单缓存
func DeleteOrderCache(ctx context.Context, orderID, userID string) error {
	pipe := Client.Pipeline()
	pipe.Del(ctx, orderCacheKey(orderID))
	pipe.SRem(ctx, userOrdersKey(userID), orderID)
	_, err := pipe.Exec(ctx)
	return err
}
