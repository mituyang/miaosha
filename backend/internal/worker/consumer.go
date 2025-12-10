package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"

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
	// 订单超时 topic
	TopicOrderTimeout = "order_timeout"
	// 延迟级别 5 = 1分钟
	DelayLevel1Min = 5
)

type Consumer struct {
	cfg       *config.Config
	consumer  rocketmq.PushConsumer
	goodsRepo *repository.GoodsRepository
	orderRepo *repository.OrderRepository
}

func NewConsumer(cfg *config.Config) *Consumer {
	return &Consumer{
		cfg:       cfg,
		goodsRepo: repository.NewGoodsRepository(database.DB),
		orderRepo: repository.NewOrderRepository(database.DB),
	}
}

// Start 启动消费者
func (c *Consumer) Start() error {
	// 初始化 MQ Producer (用于发送延迟消息)
	if err := mq.InitProducer(&c.cfg.RocketMQ); err != nil {
		return fmt.Errorf("init producer failed: %w", err)
	}

	pc, err := rocketmq.NewPushConsumer(
		consumer.WithNameServer([]string{c.cfg.RocketMQ.NameSrv}),
		consumer.WithGroupName(c.cfg.RocketMQ.Group),
		consumer.WithConsumeFromWhere(consumer.ConsumeFromLastOffset),
	)
	if err != nil {
		return fmt.Errorf("create consumer failed: %w", err)
	}

	// 订阅秒杀订单消息
	err = pc.Subscribe(c.cfg.RocketMQ.Topic, consumer.MessageSelector{}, c.handleMessage)
	if err != nil {
		return fmt.Errorf("subscribe seckill topic failed: %w", err)
	}

	// 订阅订单超时消息
	err = pc.Subscribe(TopicOrderTimeout, consumer.MessageSelector{}, c.handleTimeoutMessage)
	if err != nil {
		return fmt.Errorf("subscribe timeout topic failed: %w", err)
	}

	if err := pc.Start(); err != nil {
		return fmt.Errorf("start consumer failed: %w", err)
	}

	c.consumer = pc
	logger.Info.Printf("Consumer started, topics: %s, %s", c.cfg.RocketMQ.Topic, TopicOrderTimeout)
	return nil
}

// handleMessage 处理消息
func (c *Consumer) handleMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	for _, msg := range msgs {
		var seckillMsg dto.SeckillMessage
		if err := json.Unmarshal(msg.Body, &seckillMsg); err != nil {
			logger.Error.Printf("unmarshal message failed: %v", err)
			continue
		}

		if err := c.createOrder(ctx, seckillMsg.UserID, seckillMsg.GoodsID); err != nil {
			logger.Error.Printf("create order failed: userID=%d, goodsID=%d, err=%v",
				seckillMsg.UserID, seckillMsg.GoodsID, err)
			// 返回重试
			return consumer.ConsumeRetryLater, nil
		}

		logger.Info.Printf("order created: userID=%d, goodsID=%d", seckillMsg.UserID, seckillMsg.GoodsID)
	}

	return consumer.ConsumeSuccess, nil
}

// createOrder 创建订单 (事务)
func (c *Consumer) createOrder(ctx context.Context, userID, goodsID uint64) error {
	// 0. 检查是否已有有效订单（防止重复下单）
	exists, err := c.orderRepo.ExistsByUserAndGoods(userID, goodsID)
	if err != nil {
		return fmt.Errorf("check order exists failed: %w", err)
	}
	if exists {
		logger.Info.Printf("order already exists: userID=%d, goodsID=%d", userID, goodsID)
		return nil // 已有订单，视为成功
	}

	// 1. 查询商品
	goods, err := c.goodsRepo.GetByID(goodsID)
	if err != nil {
		return fmt.Errorf("get goods failed: %w", err)
	}

	// 2. 乐观锁扣减库存 (重试3次)
	var affected int64
	for i := 0; i < 3; i++ {
		affected, err = c.goodsRepo.DecrStock(goodsID, goods.Version)
		if err != nil {
			return fmt.Errorf("decr stock failed: %w", err)
		}
		if affected > 0 {
			break
		}
		// 版本冲突，重新查询
		goods, err = c.goodsRepo.GetByID(goodsID)
		if err != nil {
			return err
		}
	}

	if affected == 0 {
		return fmt.Errorf("stock not enough or version conflict")
	}

	// 3. 创建订单
	order := &model.Order{
		ID:         util.NextID(),
		UserID:     userID,
		GoodsID:    goodsID,
		PayAmount:  goods.Price,
		Status:     0,
		CreateTime: time.Now(),
	}

	if err := c.orderRepo.Create(order); err != nil {
		// 唯一索引冲突 = 重复消费，视为成功
		return nil
	}

	// 4. 发送延迟消息，1分钟后检查订单状态
	timeoutMsg := dto.OrderTimeoutMessage{
		OrderID: order.ID,
		UserID:  userID,
		GoodsID: goodsID,
	}
	body, _ := json.Marshal(timeoutMsg)
	if err := mq.SendDelayMsg(ctx, TopicOrderTimeout, body, DelayLevel1Min); err != nil {
		logger.Error.Printf("send timeout msg failed: orderID=%d, err=%v", order.ID, err)
		// 不影响订单创建，继续
	}

	return nil
}

// handleTimeoutMessage 处理订单超时消息
func (c *Consumer) handleTimeoutMessage(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	for _, msg := range msgs {
		var timeoutMsg dto.OrderTimeoutMessage
		if err := json.Unmarshal(msg.Body, &timeoutMsg); err != nil {
			logger.Error.Printf("unmarshal timeout message failed: %v", err)
			continue
		}

		if err := c.checkAndCancelOrder(ctx, timeoutMsg); err != nil {
			logger.Error.Printf("check order timeout failed: orderID=%d, err=%v", timeoutMsg.OrderID, err)
			continue
		}
	}

	return consumer.ConsumeSuccess, nil
}

// checkAndCancelOrder 检查订单状态，未支付则取消并返还库存
func (c *Consumer) checkAndCancelOrder(ctx context.Context, msg dto.OrderTimeoutMessage) error {
	// 1. 查询订单
	order, err := c.orderRepo.FindByIDAndUserID(msg.OrderID, msg.UserID)
	if err != nil {
		return fmt.Errorf("find order failed: %w", err)
	}

	// 2. 只处理未支付的订单
	if order.Status != 0 {
		logger.Info.Printf("order already processed: orderID=%d, status=%d", msg.OrderID, order.Status)
		return nil
	}

	// 3. 取消订单
	if err := c.orderRepo.UpdateStatus(msg.OrderID, 2); err != nil {
		return fmt.Errorf("cancel order failed: %w", err)
	}

	// 4. 返还 MySQL 库存
	if err := c.goodsRepo.IncrStock(msg.GoodsID); err != nil {
		logger.Error.Printf("incr mysql stock failed: goodsID=%d, err=%v", msg.GoodsID, err)
	}

	// 5. 返还 Redis 库存
	if err := redis.IncrStock(ctx, msg.GoodsID); err != nil {
		logger.Error.Printf("incr redis stock failed: goodsID=%d, err=%v", msg.GoodsID, err)
	}

	// 6. 清除用户购买记录 (允许重新抢购)
	if err := redis.ClearUserBought(ctx, msg.GoodsID, msg.UserID); err != nil {
		logger.Error.Printf("clear user bought failed: userID=%d, goodsID=%d, err=%v", msg.UserID, msg.GoodsID, err)
	}

	logger.Info.Printf("order timeout cancelled: orderID=%d, userID=%d, goodsID=%d", msg.OrderID, msg.UserID, msg.GoodsID)
	return nil
}

// Stop 停止消费者
func (c *Consumer) Stop() error {
	_ = mq.CloseProducer()
	if c.consumer != nil {
		return c.consumer.Shutdown()
	}
	return nil
}
