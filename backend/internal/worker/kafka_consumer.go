package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"gorm.io/gorm"

	"seckill/internal/config"
	"seckill/internal/dto"
	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/logger"
	"seckill/pkg/redis"
	"seckill/pkg/util"
)

const (
	// 批量写入配置
	kafkaBatchFlushInterval = 50 * time.Millisecond // 更快刷新
	kafkaBatchQueueSize     = 200000                // 增大队列容量
	kafkaBatchSize          = 1000                  // 增大批次
	kafkaConsumerCount      = 64                    // 增加到 64 个消费协程
	kafkaBatchWriterCount   = 32                    // 增加写入协程数
)

// kafkaOrderBatchItem 订单批量写入队列项
type kafkaOrderBatchItem struct {
	msg       *dto.SeckillMessage
	goods     *model.Goods
	storeTime time.Time // 从 Kafka 消费的时间
}

// KafkaConsumer Kafka 消费者
type KafkaConsumer struct {
	cfg       *config.Config
	readers   []*kafka.Reader // 多个 reader 并行消费
	goodsRepo *repository.GoodsRepository
	orderRepo *repository.OrderRepository
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// 批量写入队列
	batchQueue chan *kafkaOrderBatchItem

	// 商品缓存（避免重复查询 MySQL）
	goodsCache sync.Map
}

// NewKafkaConsumer 创建 Kafka 消费者
func NewKafkaConsumer(cfg *config.Config) *KafkaConsumer {
	return &KafkaConsumer{
		cfg:        cfg,
		readers:    make([]*kafka.Reader, 0, kafkaConsumerCount),
		goodsRepo:  repository.NewGoodsRepository(database.DB),
		orderRepo:  repository.NewOrderRepository(database.DB),
		stopChan:   make(chan struct{}),
		batchQueue: make(chan *kafkaOrderBatchItem, kafkaBatchQueueSize),
	}
}

// Start 启动消费者
func (c *KafkaConsumer) Start() error {
	// 启动多个批量写入协程
	for i := 0; i < kafkaBatchWriterCount; i++ {
		c.wg.Add(1)
		go c.startBatchWriter()
	}

	// 启动多个并行消费协程，每个协程创建独立的 Reader
	// 使用 Consumer Group 机制，Kafka 会自动分配分区给不同的消费者
	for i := 0; i < kafkaConsumerCount; i++ {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:        c.cfg.Kafka.Brokers,
			Topic:          c.cfg.Kafka.Topic,
			GroupID:        c.cfg.Kafka.Group,
			MinBytes:       1,           // 最小拉取 1 字节
			MaxBytes:       10e6,        // 最大拉取 10MB
			CommitInterval: time.Second, // 自动提交间隔
			StartOffset:    kafka.LastOffset,
		})
		c.readers = append(c.readers, reader)

		c.wg.Add(1)
		go c.consumeLoop(reader, i)
	}

	logger.Info.Printf("Kafka consumer started with %d consumers, %d batch writers, brokers: %v, topic: %s, group: %s",
		kafkaConsumerCount, kafkaBatchWriterCount, c.cfg.Kafka.Brokers, c.cfg.Kafka.Topic, c.cfg.Kafka.Group)
	return nil
}

// consumeLoop 消费循环 - 批量拉取消息
func (c *KafkaConsumer) consumeLoop(reader *kafka.Reader, consumerID int) {
	defer c.wg.Done()

	const batchFetchSize = 2000 // 每次批量拉取的消息数

	for {
		select {
		case <-c.stopChan:
			return
		default:
			// 批量拉取消息
			messages := make([]kafka.Message, 0, batchFetchSize)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

			for i := 0; i < batchFetchSize; i++ {
				msg, err := reader.FetchMessage(ctx)
				if err != nil {
					break // 超时或无更多消息，退出循环
				}
				messages = append(messages, msg)
			}
			cancel()

			if len(messages) == 0 {
				continue
			}

			// 批量处理消息
			var lastMsg kafka.Message
			for _, msg := range messages {
				if err := c.handleMessage(context.Background(), &msg); err != nil {
					logger.Error.Printf("consumer[%d] handle message failed: %v", consumerID, err)
					continue
				}
				lastMsg = msg
			}

			// 提交最后一条消息的 offset
			if lastMsg.Topic != "" {
				if err := reader.CommitMessages(context.Background(), lastMsg); err != nil {
					logger.Error.Printf("consumer[%d] commit message failed: %v", consumerID, err)
				}
			}
		}
	}
}

// handleMessage 处理单条消息
func (c *KafkaConsumer) handleMessage(ctx context.Context, msg *kafka.Message) error {
	// 记录从 Kafka 消费的时间
	storeTime := time.Now()

	var seckillMsg dto.SeckillMessage
	if err := json.Unmarshal(msg.Value, &seckillMsg); err != nil {
		logger.Error.Printf("unmarshal message failed: %v", err)
		return nil // 解析失败，跳过
	}

	// 跳过空消息
	if seckillMsg.UserID == 0 || seckillMsg.GoodsID == 0 {
		return nil
	}

	return c.enqueueOrder(ctx, &seckillMsg, storeTime)
}

// enqueueOrder 将订单入队，等待批量写入（快速入队，检查延迟到写入时）
func (c *KafkaConsumer) enqueueOrder(_ context.Context, msg *dto.SeckillMessage, storeTime time.Time) error {
	// 直接入队，Redis 检查延迟到批量写入时处理
	select {
	case c.batchQueue <- &kafkaOrderBatchItem{msg: msg, goods: nil, storeTime: storeTime}:
		return nil
	default:
		return errors.New("order batch queue full")
	}
}

// startBatchWriter 启动批量写入协程
func (c *KafkaConsumer) startBatchWriter() {
	defer c.wg.Done()

	ticker := time.NewTicker(kafkaBatchFlushInterval)
	defer ticker.Stop()

	batch := make([]*kafkaOrderBatchItem, 0, kafkaBatchSize)

	for {
		select {
		case <-c.stopChan:
			if len(batch) > 0 {
				c.flushBatch(batch)
			}
			return
		case item := <-c.batchQueue:
			batch = append(batch, item)
			// 达到批量大小时立即刷新
			if len(batch) >= kafkaBatchSize {
				c.flushBatch(batch)
				batch = make([]*kafkaOrderBatchItem, 0, kafkaBatchSize)
			}
		case <-ticker.C:
			if len(batch) > 0 {
				c.flushBatch(batch)
				batch = make([]*kafkaOrderBatchItem, 0, kafkaBatchSize)
			}
		}
	}
}

// flushBatch 批量写入 MySQL
func (c *KafkaConsumer) flushBatch(batch []*kafkaOrderBatchItem) {
	if len(batch) == 0 {
		return
	}

	// 按商品ID分组
	groupByGoods := make(map[uint64][]*kafkaOrderBatchItem)
	for _, item := range batch {
		groupByGoods[item.msg.GoodsID] = append(groupByGoods[item.msg.GoodsID], item)
	}

	for goodsID, items := range groupByGoods {
		c.flushGoodsBatch(goodsID, items)
	}
}

// flushGoodsBatch 批量写入单个商品的订单
func (c *KafkaConsumer) flushGoodsBatch(goodsID uint64, items []*kafkaOrderBatchItem) {
	ctx := context.Background()
	writeTime := time.Now()

	// 获取商品信息（缓存）
	goods, err := c.getGoodsCached(goodsID)
	if err != nil {
		logger.Error.Printf("get goods failed: goodsID=%d, err=%v", goodsID, err)
		return
	}

	// 批量 Redis 幂等检查
	userIDs := make([]uint64, len(items))
	for i, item := range items {
		userIDs[i] = item.msg.UserID
	}

	checkResults, err := redis.CheckProcessedBatch(ctx, goodsID, userIDs)
	if err != nil {
		logger.Error.Printf("batch check processed failed: goodsID=%d, err=%v", goodsID, err)
		return
	}

	// 过滤有效订单
	validItems := make([]*kafkaOrderBatchItem, 0, len(items))
	for _, item := range items {
		if checkResults[item.msg.UserID] == 1 {
			validItems = append(validItems, item)
		}
	}

	if len(validItems) == 0 {
		return
	}

	// 构建订单列表
	orders := make([]*model.Order, 0, len(validItems))
	for _, item := range validItems {
		msg := item.msg
		requestTime := time.Now()
		createTime := time.Now()
		if msg.RequestTime > 0 {
			requestTime = time.UnixMilli(msg.RequestTime)
		}
		if msg.CreateTime > 0 {
			createTime = time.UnixMilli(msg.CreateTime)
		}

		// 解析 born_time（进入 Kafka 的时间）
		bornTime := time.Now()
		if msg.BornTime > 0 {
			bornTime = time.UnixMilli(msg.BornTime)
		}

		orders = append(orders, &model.Order{
			ID:          util.NextID(),
			UserID:      msg.UserID,
			GoodsID:     goodsID,
			PayAmount:   goods.Price,
			Status:      0,
			RequestTime: requestTime,
			CreateTime:  createTime,
			BornTime:    bornTime,       // 进入 Kafka 时间
			StoreTime:   item.storeTime, // 从 Kafka 消费时间
			WriteTime:   writeTime,
		})
	}

	items = validItems // 更新 items 为有效项

	// 事务：批量扣库存 + 批量创建订单
	var affected int64
	err = c.orderRepo.GetDB().Transaction(func(tx *gorm.DB) error {
		var txErr error
		affected, txErr = c.goodsRepo.DecrStockBatchWithTx(tx, goodsID, len(orders))
		if txErr != nil {
			return txErr
		}

		if affected == 0 {
			return ErrStockNotEnough
		}
		if int(affected) < len(orders) {
			orders = orders[:affected]
		}

		if err := c.orderRepo.BatchCreateWithTx(tx, orders); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, ErrStockNotEnough) {
			logger.Error.Printf("MySQL stock not enough: goodsID=%d, count=%d", goodsID, len(items))
			// 清理 bought 标记、processed 标记，并返还 Redis 库存
			for _, item := range items {
				_ = redis.ClearUserBought(ctx, goodsID, item.msg.UserID)
				_ = redis.ClearProcessed(ctx, goodsID, item.msg.UserID)
				_ = redis.IncrSegmentStock(ctx, goodsID, item.msg.SegmentID)
			}
		} else if !isDuplicateKeyError(err) {
			for _, item := range items {
				_ = redis.ClearProcessed(ctx, goodsID, item.msg.UserID)
			}
			logger.Error.Printf("batch write failed: goodsID=%d, err=%v", goodsID, err)
		}
		return
	}

	// 批量添加订单到超时队列 (Redis ZSET)
	timeoutSeconds := c.cfg.Timeout.OrderTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}

	timeoutItems := make([]redis.OrderTimeoutItem, len(orders))
	for i, order := range orders {
		timeoutItems[i] = redis.OrderTimeoutItem{
			OrderID:   order.ID,
			UserID:    order.UserID,
			GoodsID:   goodsID,
			SegmentID: items[i].msg.SegmentID,
		}
	}
	expireAt := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	if err := redis.AddOrderTimeoutBatch(ctx, timeoutItems, expireAt); err != nil {
		logger.Error.Printf("batch add order timeout failed: goodsID=%d, err=%v", goodsID, err)
	}

	logger.Info.Printf("batch write success: goodsID=%d, count=%d", goodsID, len(orders))
}

// ErrStockNotEnough 库存不足错误
var ErrStockNotEnough = errors.New("stock not enough")

// getGoodsCached 从缓存获取商品信息
func (c *KafkaConsumer) getGoodsCached(goodsID uint64) (*model.Goods, error) {
	// 先查缓存
	if cached, ok := c.goodsCache.Load(goodsID); ok {
		return cached.(*model.Goods), nil
	}

	// 缓存未命中，查 MySQL
	goods, err := c.goodsRepo.GetByID(goodsID)
	if err != nil {
		return nil, err
	}

	// 存入缓存
	c.goodsCache.Store(goodsID, goods)
	return goods, nil
}

// isDuplicateKeyError 判断是否是唯一索引冲突错误
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "1062") ||
		strings.Contains(errStr, "Duplicate entry")
}

// Stop 停止消费者
func (c *KafkaConsumer) Stop() error {
	close(c.stopChan)
	c.wg.Wait()

	// 关闭所有 readers
	for i, reader := range c.readers {
		if reader != nil {
			if err := reader.Close(); err != nil {
				logger.Error.Printf("close kafka reader[%d] failed: %v", i, err)
			}
		}
	}

	logger.Info.Printf("Kafka consumer stopped, closed %d readers", len(c.readers))
	return nil
}
