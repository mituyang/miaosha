package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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

	// 超时消费者重试配置
	timeoutConsumerMaxRetries = 5
	timeoutConsumerRetryDelay = 3 * time.Second

	// 超时消息队列配置
	timeoutMsgQueueSize  = 10000           // 队列容量
	timeoutMsgSendRate   = 5000            // 每秒发送数量
	timeoutMsgRetryDelay = 5 * time.Second // 发送失败重试间隔
	timeoutMsgMaxRetries = 10              // 单条消息最大重试次数

	// 默认订单超时时间（秒）
	defaultOrderTimeoutSeconds = 60

	// 批量写入配置
	batchFlushInterval = 100 * time.Millisecond // 批量刷盘间隔
	batchQueueSize     = 10000                  // 批量队列容量
)

// timeoutMsgItem 超时消息队列项
type timeoutMsgItem struct {
	body        []byte
	orderID     uint64
	deliverTime time.Time // 精确投递时间
	retries     int
}

// orderBatchItem 订单批量写入队列项
type orderBatchItem struct {
	msg       *dto.SeckillMessage
	goods     *model.Goods
	bornTime  int64 // MQ Producer发送时间(毫秒)
	storeTime int64 // MQ Broker存储时间(毫秒)
}

// 全局超时消息队列
var timeoutMsgQueue = make(chan *timeoutMsgItem, timeoutMsgQueueSize)

// 全局订单批量写入队列
var orderBatchQueue = make(chan *orderBatchItem, batchQueueSize)

type Consumer struct {
	cfg             *config.Config
	consumer        rocketmq.PushConsumer
	timeoutConsumer rocketmq.PushConsumer
	goodsRepo       *repository.GoodsRepository
	orderRepo       *repository.OrderRepository
	stopChan        chan struct{}
	wg              sync.WaitGroup
}

func NewConsumer(cfg *config.Config) *Consumer {
	return &Consumer{
		cfg:       cfg,
		goodsRepo: repository.NewGoodsRepository(database.DB),
		orderRepo: repository.NewOrderRepository(database.DB),
		stopChan:  make(chan struct{}),
	}
}

func (c *Consumer) Start() error {
	if err := mq.InitProducer(&c.cfg.RocketMQ); err != nil {
		return fmt.Errorf("init producer failed: %w", err)
	}

	// 确保 topic 存在（通过发送消息触发自动创建）
	ctx := context.Background()
	if err := mq.EnsureTopic(ctx, c.cfg.RocketMQ.Topic); err != nil {
		logger.Info.Printf("ensure topic %s: %v (may already exist)", c.cfg.RocketMQ.Topic, err)
	}
	if err := mq.EnsureTopic(ctx, TopicOrderTimeout); err != nil {
		logger.Info.Printf("ensure topic %s: %v (may already exist)", TopicOrderTimeout, err)
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

	// 启动批量写入协程
	c.wg.Add(1)
	go c.startBatchWriter()

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

		// 使用定时消息，指定精确投递时间
		err := mq.SendTimerMsg(context.Background(), TopicOrderTimeout, item.body, item.deliverTime)
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
			consumer.WithConsumeGoroutineNums(256),
			consumer.WithPullBatchSize(32),
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

		// 跳过空消息（EnsureTopic 发送的初始化消息）
		if seckillMsg.UserID == 0 || seckillMsg.GoodsID == 0 {
			continue
		}

		if err := c.enqueueOrder(ctx, &seckillMsg, msg.BornTimestamp, msg.StoreTimestamp); err != nil {
			logger.Error.Printf("enqueue order failed: userID=%d, goodsID=%d, err=%v",
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

// enqueueOrder 将订单入队，等待批量写入
func (c *Consumer) enqueueOrder(ctx context.Context, msg *dto.SeckillMessage, bornTime, storeTime int64) error {
	userID, goodsID := msg.UserID, msg.GoodsID

	// 1. 幂等检查
	checkResult, err := redis.CheckProcessed(ctx, goodsID, userID)
	if err != nil {
		return fmt.Errorf("check processed failed: %w", err)
	}

	switch checkResult {
	case -1:
		logger.Error.Printf("user not marked: userID=%d, goodsID=%d", userID, goodsID)
		return nil
	case -2:
		return nil
	}

	// 2. 查询商品（获取价格）
	goods, err := c.goodsRepo.GetByID(goodsID)
	if err != nil {
		_ = redis.ClearProcessed(ctx, goodsID, userID)
		return fmt.Errorf("get goods failed: %w", err)
	}

	// 3. 入队等待批量写入
	select {
	case orderBatchQueue <- &orderBatchItem{msg: msg, goods: goods, bornTime: bornTime, storeTime: storeTime}:
		return nil
	default:
		// 队列满，清除标记允许重试
		_ = redis.ClearProcessed(ctx, goodsID, userID)
		return fmt.Errorf("order batch queue full")
	}
}

// startBatchWriter 启动批量写入协程
func (c *Consumer) startBatchWriter() {
	defer c.wg.Done()

	ticker := time.NewTicker(batchFlushInterval)
	defer ticker.Stop()

	batch := make([]*orderBatchItem, 0, 1000)

	for {
		select {
		case <-c.stopChan:
			// 停止前刷盘剩余数据
			if len(batch) > 0 {
				c.flushBatch(batch)
			}
			return
		case item := <-orderBatchQueue:
			batch = append(batch, item)
		case <-ticker.C:
			if len(batch) > 0 {
				c.flushBatch(batch)
				batch = make([]*orderBatchItem, 0, 1000)
			}
		}
	}
}

// flushBatch 批量写入 MySQL
func (c *Consumer) flushBatch(batch []*orderBatchItem) {
	if len(batch) == 0 {
		return
	}

	// 按商品ID分组，减少锁竞争
	groupByGoods := make(map[uint64][]*orderBatchItem)
	for _, item := range batch {
		groupByGoods[item.msg.GoodsID] = append(groupByGoods[item.msg.GoodsID], item)
	}

	// 逐个商品处理
	for goodsID, items := range groupByGoods {
		c.flushGoodsBatch(goodsID, items)
	}
}

// flushGoodsBatch 批量写入单个商品的订单
func (c *Consumer) flushGoodsBatch(goodsID uint64, items []*orderBatchItem) {
	ctx := context.Background()

	// 构建订单列表
	writeTime := time.Now() // MySQL写入时间
	orders := make([]*model.Order, 0, len(items))
	for _, item := range items {
		msg := item.msg
		requestTime := time.Now()
		createTime := time.Now()
		bornTime := time.Now()
		storeTime := time.Now()
		if msg.RequestTime > 0 {
			requestTime = time.UnixMilli(msg.RequestTime)
		}
		if msg.CreateTime > 0 {
			createTime = time.UnixMilli(msg.CreateTime)
		}
		if item.bornTime > 0 {
			bornTime = time.UnixMilli(item.bornTime)
		}
		if item.storeTime > 0 {
			storeTime = time.UnixMilli(item.storeTime)
		}
		orders = append(orders, &model.Order{
			ID:          util.NextID(),
			UserID:      msg.UserID,
			GoodsID:     goodsID,
			PayAmount:   item.goods.Price,
			Status:      0,
			RequestTime: requestTime,
			CreateTime:  createTime,
			BornTime:    bornTime,
			StoreTime:   storeTime,
			WriteTime:   writeTime,
		})
	}

	// 事务：批量扣库存 + 批量创建订单
	var affected int64
	err := c.orderRepo.GetDB().Transaction(func(tx *gorm.DB) error {
		// 批量扣减库存（带行锁）
		var err error
		affected, err = c.goodsRepo.DecrStockBatchWithTx(tx, goodsID, len(orders))
		if err != nil {
			return fmt.Errorf("batch decr stock failed: %w", err)
		}

		// 只创建成功扣减库存数量的订单
		if affected == 0 {
			return ErrStockNotEnough
		}
		if int(affected) < len(orders) {
			orders = orders[:affected]
		}

		// 批量创建订单
		if err := c.orderRepo.BatchCreateWithTx(tx, orders); err != nil {
			return fmt.Errorf("batch create orders failed: %w", err)
		}
		return nil
	})

	// 处理结果
	if err != nil {
		if errors.Is(err, ErrStockNotEnough) {
			logger.Error.Printf("MySQL stock not enough: goodsID=%d, count=%d", goodsID, len(items))
		} else if !isDuplicateKeyError(err) {
			// 非幂等错误，清除标记允许重试
			for _, item := range items {
				_ = redis.ClearProcessed(ctx, goodsID, item.msg.UserID)
			}
			logger.Error.Printf("batch write failed: goodsID=%d, err=%v", goodsID, err)
		}
		return
	}

	// 发送超时消息
	timeoutSeconds := c.cfg.RocketMQ.OrderTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultOrderTimeoutSeconds
	}

	for i, order := range orders {
		item := items[i]
		deliverTime := order.CreateTime.Add(time.Duration(timeoutSeconds) * time.Second)

		timeoutMsg := dto.OrderTimeoutMessage{
			OrderID:   order.ID,
			UserID:    item.msg.UserID,
			GoodsID:   goodsID,
			SegmentID: item.msg.SegmentID,
		}
		body, _ := json.Marshal(timeoutMsg)

		select {
		case timeoutMsgQueue <- &timeoutMsgItem{body: body, orderID: order.ID, deliverTime: deliverTime, retries: 0}:
		default:
			go func(orderID uint64, msgBody []byte, dt time.Time) {
				if err := mq.SendTimerMsg(context.Background(), TopicOrderTimeout, msgBody, dt); err != nil {
					logger.Error.Printf("send timeout message failed: orderID=%d, err=%v", orderID, err)
				}
			}(order.ID, body, deliverTime)
		}
	}

	logger.Info.Printf("batch write success: goodsID=%d, count=%d", goodsID, len(orders))
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

		// 跳过空消息（EnsureTopic 发送的初始化消息）
		if timeoutMsg.OrderID == 0 {
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
	affected, err := c.orderRepo.CancelOrder(msg.OrderID, time.Now())
	if err != nil {
		return fmt.Errorf("cancel order failed: %w", err)
	}

	// affected=0 表示订单不存在或已经不是待支付状态，无需处理
	if affected == 0 {
		return nil
	}

	// 返还库存（订单确实被取消了才返还）
	_ = c.goodsRepo.IncrStock(msg.GoodsID)
	_ = redis.IncrSegmentStock(ctx, msg.GoodsID, msg.SegmentID)

	// 清除用户标记，允许重新抢购
	_ = redis.ClearUserBought(ctx, msg.GoodsID, msg.UserID)
	_ = redis.ClearProcessed(ctx, msg.GoodsID, msg.UserID)

	logger.Info.Printf("order cancelled: orderID=%d", msg.OrderID)
	return nil
}

func (c *Consumer) Stop() error {
	// 通知批量写入协程停止
	close(c.stopChan)
	// 等待批量写入完成
	c.wg.Wait()

	_ = mq.CloseProducer()
	if c.timeoutConsumer != nil {
		_ = c.timeoutConsumer.Shutdown()
	}
	if c.consumer != nil {
		return c.consumer.Shutdown()
	}
	return nil
}
