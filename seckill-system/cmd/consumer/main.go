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
func processExpiredOrders(ctx context.Context, db *gorm.DB) {
	// 从 Redis 获取已过期的订单（最多处理 100 条）
	orderIDs, err := redis.GetExpiredOrders(ctx, 100)
	if err != nil {
		log.Printf("获取过期订单失败: %v", err)
		return
	}

	if len(orderIDs) == 0 {
		return // 没有过期订单，直接返回
	}

	log.Printf("发现 %d 个过期订单，开始处理...", len(orderIDs))

	for _, orderID := range orderIDs {
		cancelExpiredOrder(ctx, db, orderID)
	}
}

// cancelExpiredOrder 取消单个过期订单
// 优先从 Redis 缓存获取订单信息，发送 Kafka 消息更新 MySQL
func cancelExpiredOrder(ctx context.Context, db *gorm.DB, orderID string) {
	var userID string
	var goodsID int64
	var currentStatus int8

	// 1. 先从 Redis 缓存查询订单
	orderCache, err := redis.GetOrderCache(ctx, orderID)
	if err == nil && orderCache != nil {
		userID = orderCache.UserID
		goodsID = orderCache.GoodsID
		currentStatus = orderCache.Status
	} else {
		// 2. 缓存没有，查 MySQL
		var order model.SeckillOrder
		if err := db.Where("order_id = ?", orderID).First(&order).Error; err != nil {
			// 订单不存在，直接从队列移除
			if removeErr := redis.RemoveFromDelayQueue(ctx, orderID); removeErr != nil {
				log.Printf("移除延迟队列失败: orderID=%s, error=%v", orderID, removeErr)
			}
			return
		}
		userID = order.UserID
		goodsID = order.GoodsID
		currentStatus = order.Status
	}

	// 3. 只处理待支付状态的订单
	if currentStatus != 0 {
		// 订单已支付或已取消，从队列移除
		if removeErr := redis.RemoveFromDelayQueue(ctx, orderID); removeErr != nil {
			log.Printf("移除延迟队列失败: orderID=%s, error=%v", orderID, removeErr)
		}
		return
	}

	// 4. 更新 Redis 缓存状态为已取消
	if updateErr := redis.UpdateOrderStatus(ctx, orderID, 2); updateErr != nil {
		log.Printf("更新订单缓存状态失败: orderID=%s, error=%v", orderID, updateErr)
	}

	// 5. 恢复 Redis 库存和已购记录
	stockKey := fmt.Sprintf("seckill:stock:%d", goodsID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", goodsID)
	pipe := redis.Client.Pipeline()
	pipe.Incr(ctx, stockKey)
	pipe.SRem(ctx, boughtKey, userID)
	if _, pipeErr := pipe.Exec(ctx); pipeErr != nil {
		log.Printf("恢复 Redis 库存失败: orderID=%s, error=%v", orderID, pipeErr)
	}

	// 6. 从延迟队列移除
	if removeErr := redis.RemoveFromDelayQueue(ctx, orderID); removeErr != nil {
		log.Printf("移除延迟队列失败: orderID=%s, error=%v", orderID, removeErr)
	}

	// 7. 发送 Kafka 消息更新 MySQL
	if sendErr := kafka.SendOrderStatusUpdate(ctx, orderID, userID, goodsID, 2); sendErr != nil {
		log.Printf("发送取消状态更新消息失败: orderID=%s, error=%v", orderID, sendErr)
	}

	log.Printf("订单超时取消成功: orderID=%s, userID=%s, goodsID=%d", orderID, userID, goodsID)
}
