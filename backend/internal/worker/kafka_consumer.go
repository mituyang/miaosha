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

// kafkaOrderBatchItem 订单批量写入队列项
type kafkaOrderBatchItem struct {
	msg       *dto.SeckillMessage
	goods     *model.Goods
	bornTime  time.Time     // 生产者发送时间（从 kafka.Message.Time 获取）
	storeTime time.Time     // 从 Kafka 消费的时间
	reader    *kafka.Reader // 消息来源的 reader，用于提交 offset
	kafkaMsg  kafka.Message // 原始 Kafka 消息，用于提交 offset
}

// cachedGoods 带过期时间的商品缓存
type cachedGoods struct {
	goods    *model.Goods
	expireAt time.Time
}

// 商品缓存TTL
const goodsCacheTTL = 5 * time.Minute

// KafkaConsumer Kafka 消费者
type KafkaConsumer struct {
	cfg       *config.Config
	kafkaCfg  config.KafkaConsumerConfig // Consumer 配置
	readers   []*kafka.Reader            // 多个 reader 并行消费
	goodsRepo *repository.GoodsRepository
	orderRepo *repository.OrderRepository
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// 批量写入队列
	batchQueue chan *kafkaOrderBatchItem

	// 商品缓存（避免重复查询 MySQL，带TTL）
	goodsCache sync.Map

	// 死信队列 writer
	dlqWriter *kafka.Writer
}

// NewKafkaConsumer 创建 Kafka 消费者
func NewKafkaConsumer(cfg *config.Config) *KafkaConsumer {
	kafkaCfg := cfg.Kafka.Consumer

	// 校验配置
	if kafkaCfg.ConsumerCount <= 0 {
		panic("kafka consumer config error: consumer_count must be > 0")
	}
	if kafkaCfg.BatchWriterCount <= 0 {
		panic("kafka consumer config error: batch_writer_count must be > 0")
	}
	if kafkaCfg.BatchSize <= 0 {
		panic("kafka consumer config error: batch_size must be > 0")
	}
	if kafkaCfg.BatchQueueSize <= 0 {
		panic("kafka consumer config error: batch_queue_size must be > 0")
	}
	if kafkaCfg.BatchFlushMs <= 0 {
		panic("kafka consumer config error: batch_flush_ms must be > 0")
	}
	if kafkaCfg.FetchBatchSize <= 0 {
		panic("kafka consumer config error: fetch_batch_size must be > 0")
	}
	if kafkaCfg.FetchTimeoutMs <= 0 {
		panic("kafka consumer config error: fetch_timeout_ms must be > 0")
	}
	if kafkaCfg.MinBytes <= 0 {
		panic("kafka consumer config error: min_bytes must be > 0")
	}
	if kafkaCfg.MaxBytes <= 0 {
		panic("kafka consumer config error: max_bytes must be > 0")
	}

	// 创建死信队列 writer
	dlqWriter := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Kafka.Brokers...),
		Topic:    cfg.Kafka.Topic + "_dlq",
		Balancer: &kafka.Hash{},
	}

	return &KafkaConsumer{
		cfg:        cfg,
		kafkaCfg:   kafkaCfg,
		readers:    make([]*kafka.Reader, 0, kafkaCfg.ConsumerCount),
		goodsRepo:  repository.NewGoodsRepository(database.DB),
		orderRepo:  repository.NewOrderRepository(database.DB),
		stopChan:   make(chan struct{}),
		batchQueue: make(chan *kafkaOrderBatchItem, kafkaCfg.BatchQueueSize),
		dlqWriter:  dlqWriter,
	}
}

// Start 启动消费者
func (c *KafkaConsumer) Start() error {
	// 启动多个批量写入协程
	for i := 0; i < c.kafkaCfg.BatchWriterCount; i++ {
		c.wg.Add(1)
		go c.startBatchWriter()
	}

	// 启动多个并行消费协程，每个协程创建独立的 Reader
	// 使用 Consumer Group 机制，Kafka 会自动分配分区给不同的消费者
	for i := 0; i < c.kafkaCfg.ConsumerCount; i++ {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:           c.cfg.Kafka.Brokers,
			Topic:             c.cfg.Kafka.Topic,
			GroupID:           c.cfg.Kafka.Group,
			MinBytes:          c.kafkaCfg.MinBytes,
			MaxBytes:          c.kafkaCfg.MaxBytes,
			StartOffset:       kafka.FirstOffset,
			SessionTimeout:    10 * time.Second,
			RebalanceTimeout:  5 * time.Second,
			HeartbeatInterval: 2 * time.Second,
		})
		c.readers = append(c.readers, reader)

		c.wg.Add(1)
		go c.consumeLoop(reader, i)
	}

	logger.Info.Printf("Kafka consumer started with %d consumers, %d batch writers, brokers: %v, topic: %s, group: %s",
		c.kafkaCfg.ConsumerCount, c.kafkaCfg.BatchWriterCount, c.cfg.Kafka.Brokers, c.cfg.Kafka.Topic, c.cfg.Kafka.Group)
	return nil
}

// consumeLoop 消费循环 - 批量拉取消息（不在这里提交 offset）
func (c *KafkaConsumer) consumeLoop(reader *kafka.Reader, consumerID int) {
	defer c.wg.Done()

	fetchBatchSize := c.kafkaCfg.FetchBatchSize
	fetchTimeout := time.Duration(c.kafkaCfg.FetchTimeoutMs) * time.Millisecond

	// 预分配消息切片，避免每次循环重新分配
	messages := make([]kafka.Message, 0, fetchBatchSize)

	for {
		select {
		case <-c.stopChan:
			return
		default:
			// 重置切片（复用底层数组）
			messages = messages[:0]

			// 批量拉取消息
			ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
			for i := 0; i < fetchBatchSize; i++ {
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

			// 批量处理消息，入队等待写入
			for i := range messages {
				if err := c.handleMessage(&messages[i], reader); err != nil {
					logger.Error.Printf("consumer[%d] handle message failed: %v", consumerID, err)
				}
			}
		}
	}
}

// handleMessage 处理单条消息
func (c *KafkaConsumer) handleMessage(msg *kafka.Message, reader *kafka.Reader) error {
	// 记录从 Kafka 消费的时间
	storeTime := time.Now()
	// 从 kafka.Message.Time 获取生产者发送时间（BornTime）
	bornTime := msg.Time

	var seckillMsg dto.SeckillMessage
	if err := json.Unmarshal(msg.Value, &seckillMsg); err != nil {
		logger.Error.Printf("unmarshal message failed: %v", err)
		c.sendToDLQ(msg, "unmarshal_failed")
		return nil
	}

	// 跳过空消息
	if seckillMsg.UserID == 0 || seckillMsg.GoodsID == 0 {
		return nil
	}
	if seckillMsg.ActivityID == 0 {
		if activityID, ok, err := redis.GetDefaultActivityID(context.Background(), seckillMsg.GoodsID); err == nil && ok {
			seckillMsg.ActivityID = activityID
		} else {
			seckillMsg.ActivityID = seckillMsg.GoodsID
		}
	}

	return c.enqueueOrder(&seckillMsg, bornTime, storeTime, reader, *msg)
}

// sendToDLQ 发送消息到死信队列
func (c *KafkaConsumer) sendToDLQ(msg *kafka.Message, reason string) {
	dlqMsg := kafka.Message{
		Key:   msg.Key,
		Value: msg.Value,
		Headers: []kafka.Header{
			{Key: "dlq_reason", Value: []byte(reason)},
			{Key: "original_topic", Value: []byte(msg.Topic)},
		},
	}
	if err := c.dlqWriter.WriteMessages(context.Background(), dlqMsg); err != nil {
		logger.Error.Printf("send to DLQ failed: %v", err)
	}
}

// enqueueOrder 将订单入队，等待批量写入
func (c *KafkaConsumer) enqueueOrder(msg *dto.SeckillMessage, bornTime time.Time, storeTime time.Time, reader *kafka.Reader, kafkaMsg kafka.Message) error {
	select {
	case c.batchQueue <- &kafkaOrderBatchItem{msg: msg, bornTime: bornTime, storeTime: storeTime, reader: reader, kafkaMsg: kafkaMsg}:
		return nil
	case <-c.stopChan:
		return errors.New("consumer stopped")
	}
}

// startBatchWriter 启动批量写入协程
func (c *KafkaConsumer) startBatchWriter() {
	defer c.wg.Done()

	batchFlushInterval := time.Duration(c.kafkaCfg.BatchFlushMs) * time.Millisecond
	ticker := time.NewTicker(batchFlushInterval)
	defer ticker.Stop()

	batch := make([]*kafkaOrderBatchItem, 0, c.kafkaCfg.BatchSize)

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
			if len(batch) >= c.kafkaCfg.BatchSize {
				c.flushBatch(batch)
				batch = make([]*kafkaOrderBatchItem, 0, c.kafkaCfg.BatchSize)
			}
		case <-ticker.C:
			if len(batch) > 0 {
				c.flushBatch(batch)
				batch = make([]*kafkaOrderBatchItem, 0, c.kafkaCfg.BatchSize)
			}
		}
	}
}

// flushBatch 批量写入 MySQL
func (c *KafkaConsumer) flushBatch(batch []*kafkaOrderBatchItem) {
	if len(batch) == 0 {
		return
	}

	// 按活动ID分组，Redis 幂等和库存返还都隔离到活动维度
	groupByActivity := make(map[uint64][]*kafkaOrderBatchItem)
	for _, item := range batch {
		groupByActivity[item.msg.ActivityID] = append(groupByActivity[item.msg.ActivityID], item)
	}

	// 收集成功处理的消息
	successItems := make([]*kafkaOrderBatchItem, 0, len(batch))
	for activityID, items := range groupByActivity {
		successList := c.flushActivityBatch(activityID, items)
		successItems = append(successItems, successList...)
	}

	// 按 reader 分组提交 offset
	c.commitOffsetsByReader(successItems)
}

// flushActivityBatch 批量写入单个活动的订单，返回成功处理的 items
func (c *KafkaConsumer) flushActivityBatch(activityID uint64, items []*kafkaOrderBatchItem) []*kafkaOrderBatchItem {
	ctx := context.Background()
	writeTime := time.Now()
	if len(items) == 0 {
		return nil
	}
	goodsID := items[0].msg.GoodsID

	// 获取商品信息（缓存）
	goods, err := c.getGoodsCached(goodsID)
	if err != nil {
		logger.Error.Printf("get goods failed: goodsID=%d, err=%v", goodsID, err)
		for _, item := range items {
			c.sendToDLQ(&item.kafkaMsg, "get_goods_failed")
		}
		return nil
	}

	// 批量 Redis 幂等检查
	checkItems := make([]redis.BatchCheckItem, len(items))
	for i, item := range items {
		quantity := item.msg.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		checkItems[i] = redis.BatchCheckItem{
			UserID:   item.msg.UserID,
			Quantity: quantity,
		}
	}

	checkResults, err := redis.CheckProcessedBatch(ctx, activityID, checkItems)
	if err != nil {
		logger.Error.Printf("batch check processed failed: activityID=%d, goodsID=%d, err=%v", activityID, goodsID, err)
		for _, item := range items {
			c.sendToDLQ(&item.kafkaMsg, "check_processed_failed")
		}
		return nil
	}

	// 过滤有效订单，重复消息直接算成功
	validItems := make([]*kafkaOrderBatchItem, 0, len(items))
	duplicateItems := make([]*kafkaOrderBatchItem, 0)
	for _, item := range items {
		if checkResults[item.msg.UserID] == 1 {
			validItems = append(validItems, item)
		} else {
			duplicateItems = append(duplicateItems, item)
		}
	}

	if len(validItems) == 0 {
		return duplicateItems
	}

	// 计算总库存扣减数量
	totalQuantity := 0
	for _, item := range validItems {
		quantity := item.msg.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		totalQuantity += quantity
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

		quantity := msg.Quantity
		if quantity <= 0 {
			quantity = 1
		}

		orders = append(orders, &model.Order{
			ID:          util.NextID(),
			UserID:      msg.UserID,
			GoodsID:     goodsID,
			ActivityID:  activityID,
			Quantity:    quantity,
			PayAmount:   goods.Price * float64(quantity),
			Status:      0,
			RequestTime: requestTime,
			CreateTime:  createTime,
			BornTime:    item.bornTime,
			StoreTime:   item.storeTime,
			WriteTime:   writeTime,
		})
	}

	items = validItems

	// 事务：批量扣库存 + 批量创建订单
	var affected int64
	err = c.orderRepo.GetDB().Transaction(func(tx *gorm.DB) error {
		var txErr error
		affected, txErr = c.goodsRepo.DecrStockBatchWithTx(tx, goodsID, totalQuantity)
		if txErr != nil {
			return txErr
		}

		if affected == 0 {
			return ErrStockNotEnough
		}

		if int(affected) < totalQuantity {
			cumulative := 0
			cutIndex := 0
			for i, order := range orders {
				cumulative += order.Quantity
				if cumulative > int(affected) {
					cutIndex = i
					break
				}
				cutIndex = i + 1
			}
			orders = orders[:cutIndex]
			items = items[:cutIndex]
		}

		if len(orders) == 0 {
			return ErrStockNotEnough
		}

		if err := c.orderRepo.BatchCreateWithTx(tx, orders); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, ErrStockNotEnough) {
			logger.Error.Printf("MySQL stock not enough: activityID=%d, goodsID=%d, count=%d", activityID, goodsID, len(items))
			for _, item := range items {
				quantity := item.msg.Quantity
				if quantity <= 0 {
					quantity = 1
				}
				_ = redis.ClearUserBought(ctx, activityID, item.msg.UserID, quantity)
				_ = redis.ClearProcessed(ctx, activityID, item.msg.UserID, quantity)
				_ = redis.IncrSegmentStockBy(ctx, activityID, item.msg.SegmentID, quantity)
			}
			for _, item := range items {
				c.sendToDLQ(&item.kafkaMsg, "stock_not_enough")
			}
		} else if !isDuplicateKeyError(err) {
			for _, item := range items {
				quantity := item.msg.Quantity
				if quantity <= 0 {
					quantity = 1
				}
				_ = redis.ClearProcessed(ctx, activityID, item.msg.UserID, quantity)
			}
			logger.Error.Printf("batch write failed: activityID=%d, goodsID=%d, err=%v", activityID, goodsID, err)
			for _, item := range items {
				c.sendToDLQ(&item.kafkaMsg, "batch_write_failed")
			}
		}
		return duplicateItems
	}

	// 批量添加订单到超时队列
	timeoutSeconds := c.cfg.Timeout.OrderTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}

	timeoutItems := make([]redis.OrderTimeoutItem, len(orders))
	for i, order := range orders {
		timeoutItems[i] = redis.OrderTimeoutItem{
			OrderID:    order.ID,
			UserID:     order.UserID,
			GoodsID:    goodsID,
			ActivityID: activityID,
			SegmentID:  items[i].msg.SegmentID,
			Quantity:   order.Quantity,
		}
	}
	expireAt := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	if err := redis.AddOrderTimeoutBatch(ctx, timeoutItems, expireAt); err != nil {
		logger.Error.Printf("batch add order timeout failed: activityID=%d, goodsID=%d, err=%v", activityID, goodsID, err)
	}

	_ = redis.IncrementAdminOrdersCreated(ctx, orders)
	logger.Info.Printf("batch write success: activityID=%d, goodsID=%d, count=%d", activityID, goodsID, len(orders))
	return append(items, duplicateItems...)
}

// ErrStockNotEnough 库存不足错误
var ErrStockNotEnough = errors.New("stock not enough")

// commitOffsetsByReader 按 reader 分组提交 offset
func (c *KafkaConsumer) commitOffsetsByReader(items []*kafkaOrderBatchItem) {
	if len(items) == 0 {
		return
	}

	// 按 reader 分组，每个 reader 只提交最大 offset 的消息
	readerMaxMsg := make(map[*kafka.Reader]kafka.Message)
	for _, item := range items {
		if existing, ok := readerMaxMsg[item.reader]; ok {
			if item.kafkaMsg.Offset > existing.Offset {
				readerMaxMsg[item.reader] = item.kafkaMsg
			}
		} else {
			readerMaxMsg[item.reader] = item.kafkaMsg
		}
	}

	for reader, msg := range readerMaxMsg {
		if err := reader.CommitMessages(context.Background(), msg); err != nil {
			logger.Error.Printf("commit offset failed: partition=%d, offset=%d, err=%v", msg.Partition, msg.Offset, err)
		}
	}
}

// getGoodsCached 从缓存获取商品信息（带TTL）
func (c *KafkaConsumer) getGoodsCached(goodsID uint64) (*model.Goods, error) {
	// 先查缓存
	if cached, ok := c.goodsCache.Load(goodsID); ok {
		cg := cached.(*cachedGoods)
		// 检查是否过期
		if time.Now().Before(cg.expireAt) {
			return cg.goods, nil
		}
		// 已过期，删除缓存
		c.goodsCache.Delete(goodsID)
	}

	// 缓存未命中或已过期，查 MySQL
	goods, err := c.goodsRepo.GetByID(goodsID)
	if err != nil {
		return nil, err
	}

	// 存入缓存（带过期时间）
	c.goodsCache.Store(goodsID, &cachedGoods{
		goods:    goods,
		expireAt: time.Now().Add(goodsCacheTTL),
	})
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

	for i, reader := range c.readers {
		if reader != nil {
			if err := reader.Close(); err != nil {
				logger.Error.Printf("close kafka reader[%d] failed: %v", i, err)
			}
		}
	}

	if c.dlqWriter != nil {
		if err := c.dlqWriter.Close(); err != nil {
			logger.Error.Printf("close DLQ writer failed: %v", err)
		}
	}

	logger.Info.Printf("Kafka consumer stopped, closed %d readers", len(c.readers))
	return nil
}
