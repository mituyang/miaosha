package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"seckill-system/internal/model"
	"seckill-system/pkg/config"
	"seckill-system/pkg/kafka"
	"seckill-system/pkg/redis"
)

// 注意：model 包仍然被 Kafka Consumer 使用

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化数据库连接
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	log.Println("数据库连接成功")

	// 自动迁移表结构
	if err := db.AutoMigrate(&model.SeckillGoods{}, &model.SeckillOrder{}); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 初始化 Redis 连接（用于延迟队列）
	if err := redis.InitRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB); err != nil {
		log.Fatalf("连接 Redis 失败: %v", err)
	}
	log.Println("Redis 连接成功")

	// 初始化 Kafka Producer（用于发送状态更新消息）
	kafka.InitProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer kafka.Close()
	log.Println("Kafka Producer 初始化成功")

	// 创建 Kafka 消费者
	consumer := kafka.NewConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.Topic,
		cfg.Kafka.GroupID,
		db,
	)
	defer consumer.Close()

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 监听系统信号，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("收到退出信号，正在关闭...")
		cancel()
	}()

	// 启动延迟队列消费者（处理超时订单）
	go startDelayQueueConsumer(ctx, db)

	// 启动 Kafka 消费者
	consumer.Start(ctx)
}

// startDelayQueueConsumer 启动延迟队列消费者
// 每秒从 Redis ZSET 中获取已过期的订单并取消
func startDelayQueueConsumer(ctx context.Context, db *gorm.DB) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	log.Println("延迟队列消费者启动...")

	for {
		select {
		case <-ctx.Done():
			log.Println("延迟队列消费者退出")
			return
		case <-ticker.C:
			processExpiredOrders(ctx, db)
		}
	}
}

// processExpiredOrders 处理已过期的订单
// 批量处理：一次性获取所有过期订单，批量更新 Redis，批量发送 Kafka
func processExpiredOrders(ctx context.Context, db *gorm.DB) {
	// 1. 从 Redis 获取所有已过期的订单
	orderIDs, err := redis.GetExpiredOrders(ctx)
	if err != nil {
		log.Printf("获取过期订单失败: %v", err)
		return
	}

	if len(orderIDs) == 0 {
		return
	}

	log.Printf("发现 %d 个过期订单，开始批量处理...", len(orderIDs))

	// 2. 批量获取订单缓存信息
	type orderInfo struct {
		orderID string
		userID  string
		goodsID int64
	}
	var toCancel []orderInfo

	for _, orderID := range orderIDs {
		orderCache, cacheErr := redis.GetOrderCache(ctx, orderID)
		if cacheErr == nil && orderCache != nil {
			if orderCache.Status == 0 { // 只处理待支付的
				toCancel = append(toCancel, orderInfo{
					orderID: orderID,
					userID:  orderCache.UserID,
					goodsID: orderCache.GoodsID,
				})
			}
		}
		// 无论如何都从延迟队列移除
		redis.RemoveFromDelayQueue(ctx, orderID)
	}

	if len(toCancel) == 0 {
		return
	}

	// 3. 批量更新 Redis（Pipeline 一次性执行）
	pipe := redis.Client.Pipeline()
	for _, o := range toCancel {
		// 更新缓存状态
		cacheKey := fmt.Sprintf("order:cache:%s", o.orderID)
		// 恢复库存
		stockKey := fmt.Sprintf("seckill:stock:%d", o.goodsID)
		boughtKey := fmt.Sprintf("seckill:bought:%d", o.goodsID)

		pipe.Incr(ctx, stockKey)
		pipe.SRem(ctx, boughtKey, o.userID)
		pipe.Del(ctx, cacheKey) // 删除缓存，让后续查询走 MySQL
	}
	if _, pipeErr := pipe.Exec(ctx); pipeErr != nil {
		log.Printf("批量更新 Redis 失败: %v", pipeErr)
	}

	// 4. 批量发送 Kafka 消息
	for _, o := range toCancel {
		if sendErr := kafka.SendOrderStatusUpdate(ctx, o.orderID, o.userID, o.goodsID, 2); sendErr != nil {
			log.Printf("发送取消消息失败: orderID=%s, error=%v", o.orderID, sendErr)
		}
	}

	log.Printf("批量取消 %d 个超时订单完成", len(toCancel))
}
