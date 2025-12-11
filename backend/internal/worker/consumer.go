package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"gorm.io/gorm"

	"seckill/internal/config"
	"seckill/internal/dto"
	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/mq"
	"seckill/pkg/redis"
	"seckill/pkg/util"
)

const (
	TopicOrderTimeout = "order_timeout"
	DelayLevel1Min    = 5

	// 超时消费者重试配置
	timeoutConsumerMaxRetries = 5
	timeoutConsumerRetryDelay = 3 * time.Second

	// 超时消息队列配置
	timeoutMsgQueueSize  = 10000           // 队列容量
	timeoutMsgSendRate   = 500             // 每秒发送数量
	timeoutMsgRetryDelay = 5 * time.Second // 发送失败重试间隔
	timeoutMsgMaxRetries = 10              // 单条消息最大重试次数
)

// timeoutMsgItem 超时消息队列项
type timeoutMsgItem struct {
	body    []byte
	orderID uint64
	retries int
}

// 全局超时消息队列
var timeoutMsgQueue = make(chan *timeoutMsgItem, timeoutMsgQueueSize)

type Consumer struct {
	cfg             *config.Config
	consumer        rocketmq.PushConsumer
	timeoutConsumer rocketmq.PushConsumer
	goodsRepo       *repository.GoodsRepository
	orderRepo       *repository.OrderRepository
}

func NewConsumer(cfg *config.Config) *Consumer {
	return &Consumer{
		cfg:       cfg,
		goodsRepo: repository.NewGoodsRepository(database.DB),
		orderRepo: repository.NewOrderRepository(database.DB),
	}
}

func (c *Consumer) Start() error {
	if err := mq.InitProducer(&c.cfg.RocketMQ); err != nil {
		return fmt.Errorf("init producer failed: %w", err)
	}

	pc, err := rocketmq.NewPushConsumer(
		consumer.WithNameServer([]string{c.cfg.RocketMQ.NameSrv}),
		consumer.WithGroupName(c.cfg.RocketMQ.Group),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromLastOffset),
		consumer.WithConsumeGoroutineNums(128),
		consumer.WithPullBatchSize(32),
	)
	if err != nil {
		return fmt.Errorf("create consumer failed: %w", err)
	}

	if err := pc.Subscribe(c.cfg.RocketMQ.Topic, consumer.MessageSelector{}, c.handleMessage); err != nil {
		return fmt.Errorf("subscribe seckill topic failed: %w", err)
	}

	if err := pc.Start(); err != nil {
		return fmt.Errorf("start consumer failed: %w", err)
	}

	c.consumer = pc

	// 启动超时消费者（带重试）
	go c.startTimeoutConsumerWithRetry()

	// 启动超时消息发送协程
	go c.startTimeoutMsgSender()

	logger.Info.Printf("Consumer started")
	return nil
}

// startTimeoutMsgSender 启动超时消息发送协程（限速发送，削峰填谷）
func (c *Consumer) startTimeoutMsgSender() {
	// 使用 ticker 控制发送速率
	interval := time.Second / timeoutMsgSendRate
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info.Printf("timeout message sender started, rate: %d/s", timeoutMsgSendRate)

	for item := range timeoutMsgQueue {
		<-ticker.C // 限速

		err := mq.SendDelayMsg(context.Background(), TopicOrderTimeout, item.body, DelayLevel1Min)
		if err != nil {
			item.retries++
			if item.retries < timeoutMsgMaxRetries {
				// 重新入队，稍后重试
				go func(it *timeoutMsgItem) {
					time.Sleep(timeoutMsgRetryDelay)
					select {
					case timeoutMsgQueue <- it:
					default:
						logger.Error.Printf("timeout msg queue full, drop message: orderID=%d", it.orderID)
					}
				}(item)
			} else {
				logger.Error.Printf("timeout msg send failed after %d retries: orderID=%d, err=%v",
					item.retries, item.orderID, err)
			}
		}
	}
}

// startTimeoutConsumerWithRetry 带重试的超时消费者启动
func (c *Consumer) startTimeoutConsumerWithRetry() {
	var lastErr error
	for i := range timeoutConsumerMaxRetries {
		if i > 0 {
			logger.Info.Printf("retrying to start timeout consumer (attempt %d/%d)...",
				i+1, timeoutConsumerMaxRetries)
			time.Sleep(timeoutConsumerRetryDelay)
		}

		pc2, err := rocketmq.NewPushConsumer(
			consumer.WithNameServer([]string{c.cfg.RocketMQ.NameSrv}),
			consumer.WithGroupName(c.cfg.RocketMQ.Group+"_timeout"),
			consumer.WithConsumeFromWhere(consumer.ConsumeFromLastOffset),
		)
		if err != nil {
			lastErr = err
			logger.Error.Printf("create timeout consumer failed: %v", err)
			continue
		}

		if err := pc2.Subscribe(TopicOrderTimeout, consumer.MessageSelector{}, c.handleTimeoutMessage); err != nil {
			lastErr = err
			logger.Error.Printf("subscribe timeout topic failed: %v", err)
			continue
		}

		if err := pc2.Start(); err != nil {
			lastErr = err
			logger.Error.Printf("start timeout consumer failed: %v", err)
			continue
		}

		c.timeoutConsumer = pc2
		logger.Info.Printf("timeout consumer started successfully")
		return
	}

	logger.Error.Printf("timeout consumer failed to start after %d attempts, last error: %v",
		timeoutConsumerMaxRetries, lastErr)
}

func (c *Consumer) handleMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	for _, msg := range msgs {
		var seckillMsg dto.SeckillMessage
		if err := json.Unmarshal(msg.Body, &seckillMsg); err != nil {
			logger.Error.Printf("unmarshal message failed: %v", err)
			continue
		}

		if err := c.createOrder(ctx, &seckillMsg); err != nil {
			logger.Error.Printf("create order failed: userID=%d, goodsID=%d, err=%v",
				seckillMsg.UserID, seckillMsg.GoodsID, err)
			return consumer.ConsumeRetryLater, nil
		}
	}
	return consumer.ConsumeSuccess, nil
}

// isDuplicateKeyError 判断是否是唯一索引冲突错误
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// MySQL 唯一索引冲突错误码 1062
	return strings.Contains(err.Error(), "1062") ||
		strings.Contains(err.Error(), "Duplicate entry")
}

func (c *Consumer) createOrder(ctx context.Context, msg *dto.SeckillMessage) error {
	userID, goodsID, segmentID := msg.UserID, msg.GoodsID, msg.SegmentID

	// 1. 扣减 Redis 库存
	decrResult, err := redis.DecrStock(ctx, goodsID, segmentID, userID)
	if err != nil {
		return fmt.Errorf("decr redis stock failed: %w", err)
	}

	switch decrResult {
	case 0:
		// Redis 库存不足，清除用户标记
		_ = redis.ClearUserBought(ctx, goodsID, userID)
		return nil
	case -1:
		// 用户未标记，可能是重复消费，检查是否已有订单
		exists, _ := c.orderRepo.ExistsByUserAndGoods(userID, goodsID)
		if exists {
			return nil // 已有订单，幂等处理
		}
		// 异常情况，记录日志
		logger.Error.Printf("user not marked but no order: userID=%d, goodsID=%d", userID, goodsID)
		return nil
	case -2:
		// 已扣过库存，重复消费，幂等处理
		return nil
	}

	// 2. 查询商品（获取价格）
	goods, err := c.goodsRepo.GetByID(goodsID)
	if err != nil {
		// 回滚 Redis 库存
		_ = redis.RollbackStock(ctx, goodsID, segmentID, userID)
		return fmt.Errorf("get goods failed: %w", err)
	}

	// 3. 构建订单
	createTime := time.Now()
	if msg.RequestTime > 0 {
		createTime = time.UnixMilli(msg.RequestTime)
	}
	order := &model.Order{
		ID:         util.NextID(),
		UserID:     userID,
		GoodsID:    goodsID,
		PayAmount:  goods.Price,
		Status:     0,
		CreateTime: createTime,
	}

	// 4. 使用事务：扣 MySQL 库存 + 创建订单
	err = c.orderRepo.GetDB().Transaction(func(tx *gorm.DB) error {
		affected, err := c.goodsRepo.DecrStockWithTx(tx, goodsID)
		if err != nil {
			return fmt.Errorf("decr stock failed: %w", err)
		}
		if affected == 0 {
			return ErrStockNotEnough
		}

		if err := c.orderRepo.CreateWithTx(tx, order); err != nil {
			return fmt.Errorf("create order failed: %w", err)
		}
		return nil
	})

	// 5. 处理事务结果
	if err != nil {
		if errors.Is(err, ErrStockNotEnough) {
			// MySQL 库存不足，回滚 Redis
			_ = redis.RollbackStock(ctx, goodsID, segmentID, userID)
			return nil
		}
		if isDuplicateKeyError(err) {
			// 重复消费，幂等处理（Redis 库存已扣，但订单已存在）
			// 需要回滚 Redis 库存
			_ = redis.RollbackStock(ctx, goodsID, segmentID, userID)
			logger.Info.Printf("duplicate order detected: userID=%d, goodsID=%d", userID, goodsID)
			return nil
		}
		// 其他错误，回滚 Redis 并重试消息
		_ = redis.RollbackStock(ctx, goodsID, segmentID, userID)
		return err
	}

	// 6. 超时消息入队（由后台协程限速发送）
	timeoutMsg := dto.OrderTimeoutMessage{
		OrderID:   order.ID,
		UserID:    userID,
		GoodsID:   goodsID,
		SegmentID: segmentID,
	}
	body, _ := json.Marshal(timeoutMsg)
	select {
	case timeoutMsgQueue <- &timeoutMsgItem{body: body, orderID: order.ID, retries: 0}:
	default:
		// 队列满，直接尝试发送（降级处理）
		go func(orderID uint64, msgBody []byte) {
			if err := mq.SendDelayMsg(context.Background(), TopicOrderTimeout, msgBody, DelayLevel1Min); err != nil {
				logger.Error.Printf("send timeout message failed (queue full): orderID=%d, err=%v", orderID, err)
			}
		}(order.ID, body)
	}

	return nil
}

// ErrStockNotEnough 库存不足错误
var ErrStockNotEnough = errors.New("stock not enough")

func (c *Consumer) handleTimeoutMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	for _, msg := range msgs {
		var timeoutMsg dto.OrderTimeoutMessage
		if err := json.Unmarshal(msg.Body, &timeoutMsg); err != nil {
			logger.Error.Printf("unmarshal timeout message failed: %v", err)
			continue
		}

		if err := c.checkAndCancelOrder(ctx, timeoutMsg); err != nil {
			logger.Error.Printf("cancel order failed: orderID=%d, err=%v", timeoutMsg.OrderID, err)
			continue
		}
	}
	return consumer.ConsumeSuccess, nil
}

func (c *Consumer) checkAndCancelOrder(ctx context.Context, msg dto.OrderTimeoutMessage) error {
	// 使用 CAS 更新，只有 status=0 才能取消，保证幂等
	affected, err := c.orderRepo.CancelOrder(msg.OrderID)
	if err != nil {
		return fmt.Errorf("cancel order failed: %w", err)
	}

	// affected=0 表示订单不存在或已经不是待支付状态，无需处理
	if affected == 0 {
		return nil
	}

	// 返还库存（订单确实被取消了才返还）
	// 注意：不清除用户标记，防止用户重复抢购导致库存不一致
	_ = c.goodsRepo.IncrStock(msg.GoodsID)
	_ = redis.IncrSegmentStock(ctx, msg.GoodsID, msg.SegmentID)

	logger.Info.Printf("order cancelled: orderID=%d", msg.OrderID)
	return nil
}

func (c *Consumer) Stop() error {
	_ = mq.CloseProducer()
	if c.timeoutConsumer != nil {
		_ = c.timeoutConsumer.Shutdown()
	}
	if c.consumer != nil {
		return c.consumer.Shutdown()
	}
	return nil
}
