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
	TopicOrderTimeout = "order_timeout"
	DelayLevel1Min    = 5
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

	if err := pc.Subscribe(TopicOrderTimeout, consumer.MessageSelector{}, c.handleTimeoutMessage); err != nil {
		logger.Error.Printf("subscribe timeout topic failed (ignored): %v", err)
	}

	if err := pc.Start(); err != nil {
		return fmt.Errorf("start consumer failed: %w", err)
	}

	c.consumer = pc
	logger.Info.Printf("Consumer started")
	return nil
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

func (c *Consumer) createOrder(ctx context.Context, msg *dto.SeckillMessage) error {
	userID, goodsID := msg.UserID, msg.GoodsID

	// 检查是否已有订单
	exists, err := c.orderRepo.ExistsByUserAndGoods(userID, goodsID)
	if err != nil {
		return fmt.Errorf("check order exists failed: %w", err)
	}
	if exists {
		return nil
	}

	// 查询商品
	goods, err := c.goodsRepo.GetByID(goodsID)
	if err != nil {
		return fmt.Errorf("get goods failed: %w", err)
	}

	// 检查库存
	if goods.Stock <= 0 {
		_ = redis.RollbackStock(ctx, goodsID, userID)
		return nil
	}

	// 扣减库存 (不用乐观锁，直接扣)
	affected, err := c.goodsRepo.DecrStockSimple(goodsID)
	if err != nil {
		return fmt.Errorf("decr stock failed: %w", err)
	}
	if affected == 0 {
		_ = redis.RollbackStock(ctx, goodsID, userID)
		return nil
	}

	// 创建订单
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

	if err := c.orderRepo.Create(order); err != nil {
		// 创建失败，返还库存
		_ = c.goodsRepo.IncrStock(goodsID)
		_ = redis.RollbackStock(ctx, goodsID, userID)
		return nil
	}

	// 发送超时消息
	timeoutMsg := dto.OrderTimeoutMessage{
		OrderID: order.ID,
		UserID:  userID,
		GoodsID: goodsID,
	}
	body, _ := json.Marshal(timeoutMsg)
	_ = mq.SendDelayMsg(ctx, TopicOrderTimeout, body, DelayLevel1Min)

	return nil
}

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
	order, err := c.orderRepo.FindByIDAndUserID(msg.OrderID, msg.UserID)
	if err != nil {
		return fmt.Errorf("find order failed: %w", err)
	}

	if order.Status != 0 {
		return nil
	}

	if err := c.orderRepo.UpdateStatus(msg.OrderID, 2); err != nil {
		return fmt.Errorf("update status failed: %w", err)
	}

	_ = c.goodsRepo.IncrStock(msg.GoodsID)
	_ = redis.IncrStock(ctx, msg.GoodsID)
	_ = redis.ClearUserBought(ctx, msg.GoodsID, msg.UserID)

	logger.Info.Printf("order cancelled: orderID=%d", msg.OrderID)
	return nil
}

func (c *Consumer) Stop() error {
	_ = mq.CloseProducer()
	if c.consumer != nil {
		return c.consumer.Shutdown()
	}
	return nil
}
