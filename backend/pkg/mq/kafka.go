package mq

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"golang.org/x/time/rate"

	"seckill/internal/config"
	"seckill/pkg/logger"
)

var (
	kafkaWriter     *kafka.Writer
	kafkaMsgBuf     chan *kafkaMsg
	kafkaStopCh     chan struct{}
	kafkaWg         sync.WaitGroup
	kafkaTopic      string
	kafkaBrokers    []string
	kafkaCfg        config.KafkaProducerConfig
	producerLimiter *rate.Limiter // 生产者限流器
)

// 最大重试次数（入队重试，非Kafka层重试）
const maxEnqueueRetries = 3

type kafkaMsg struct {
	key     []byte
	value   []byte
	retries int // 重试计数
}

// InitKafkaProducer 初始化 Kafka Producer
func InitKafkaProducer(cfg *config.KafkaConfig) error {
	kafkaBrokers = cfg.Brokers
	kafkaTopic = cfg.Topic
	kafkaCfg = cfg.Producer

	// 校验配置
	if kafkaCfg.BatchSize <= 0 {
		panic("kafka producer config error: batch_size must be > 0")
	}
	if kafkaCfg.BatchTimeoutMs <= 0 {
		panic("kafka producer config error: batch_timeout_ms must be > 0")
	}
	if kafkaCfg.BufferSize <= 0 {
		panic("kafka producer config error: buffer_size must be > 0")
	}
	if kafkaCfg.SenderCount <= 0 {
		panic("kafka producer config error: sender_count must be > 0")
	}
	if kafkaCfg.MaxRetries <= 0 {
		panic("kafka producer config error: max_retries must be > 0")
	}

	batchTimeout := time.Duration(kafkaCfg.BatchTimeoutMs) * time.Millisecond

	// 创建 Writer
	kafkaWriter = &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.Hash{}, // 按 key hash 分区
		BatchSize:    kafkaCfg.BatchSize,
		BatchTimeout: batchTimeout,
		RequiredAcks: kafka.RequireOne, // Leader 确认即可
		Async:        false,            // 同步发送，保证可靠性
		MaxAttempts:  kafkaCfg.MaxRetries,
	}

	// 初始化缓冲队列
	kafkaMsgBuf = make(chan *kafkaMsg, kafkaCfg.BufferSize)
	kafkaStopCh = make(chan struct{})

	// 初始化限流器
	if kafkaCfg.RateLimitEnabled {
		if kafkaCfg.RateLimitQPS <= 0 {
			panic("kafka producer config error: rate_limit_qps must be > 0 when rate_limit_enabled is true")
		}
		if kafkaCfg.RateLimitBurst <= 0 {
			panic("kafka producer config error: rate_limit_burst must be > 0 when rate_limit_enabled is true")
		}
		producerLimiter = rate.NewLimiter(rate.Limit(kafkaCfg.RateLimitQPS), kafkaCfg.RateLimitBurst)
		logger.Info.Printf("Kafka producer rate limiter enabled: qps=%d, burst=%d", kafkaCfg.RateLimitQPS, kafkaCfg.RateLimitBurst)
	}

	// 启动发送协程
	for i := 0; i < kafkaCfg.SenderCount; i++ {
		kafkaWg.Add(1)
		go kafkaBatchSender()
	}

	logger.Info.Printf("Kafka producer initialized, brokers: %v, topic: %s, batchSize: %d, senderCount: %d",
		cfg.Brokers, cfg.Topic, kafkaCfg.BatchSize, kafkaCfg.SenderCount)
	return nil
}

// kafkaBatchSender 批量发送协程
func kafkaBatchSender() {
	defer kafkaWg.Done()

	batchTimeout := time.Duration(kafkaCfg.BatchTimeoutMs) * time.Millisecond
	batch := make([]*kafkaMsg, 0, kafkaCfg.BatchSize)
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-kafkaStopCh:
			// 关闭前发送剩余消息
			if len(batch) > 0 {
				sendKafkaBatch(batch)
			}
			return

		case msg := <-kafkaMsgBuf:
			batch = append(batch, msg)
			if len(batch) >= kafkaCfg.BatchSize {
				sendKafkaBatch(batch)
				batch = make([]*kafkaMsg, 0, kafkaCfg.BatchSize)
				ticker.Reset(batchTimeout)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				sendKafkaBatch(batch)
				batch = make([]*kafkaMsg, 0, kafkaCfg.BatchSize)
			}
		}
	}
}

// sendKafkaBatch 发送一批消息
func sendKafkaBatch(msgs []*kafkaMsg) {
	if len(msgs) == 0 {
		return
	}

	// 限流检查：批量消息需要对应数量的令牌
	if producerLimiter != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// 为批量消息预留令牌
		if err := producerLimiter.WaitN(ctx, len(msgs)); err != nil {
			logger.Error.Printf("rate limit wait failed for batch: %v, count=%d", err, len(msgs))
			// 限流失败，重新入队
			for _, m := range msgs {
				m.retries++
				if m.retries <= maxEnqueueRetries {
					select {
					case kafkaMsgBuf <- m:
					default:
						logger.Error.Printf("kafka buffer full, drop message after rate limit, key=%s", string(m.key))
					}
				}
			}
			return
		}
	}

	// 记录发送时间（BornTime）
	bornTime := time.Now()

	// 构建 Kafka 消息
	kafkaMsgs := make([]kafka.Message, len(msgs))
	for i, m := range msgs {
		kafkaMsgs[i] = kafka.Message{
			Key:   m.key,
			Value: m.value,
			Time:  bornTime, // 真正的发送时间
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := kafkaWriter.WriteMessages(ctx, kafkaMsgs...)
	if err != nil {
		logger.Error.Printf("kafka batch send failed: %v, count=%d", err, len(msgs))
		// 失败的消息重新入队（带重试计数限制）
		for _, m := range msgs {
			m.retries++
			if m.retries > maxEnqueueRetries {
				logger.Error.Printf("kafka message dropped after %d retries, key=%s", m.retries, string(m.key))
				continue
			}
			select {
			case kafkaMsgBuf <- m:
			default:
				logger.Error.Printf("kafka buffer full, drop message, key=%s", string(m.key))
			}
		}
	}
}

// SendKafkaMsg 发送消息到 Kafka（缓冲+批量发送）
func SendKafkaMsg(ctx context.Context, key, value []byte) error {
	// 直接入队，不等待限流器
	// 限流器在发送协程中控制实际发送速率
	select {
	case kafkaMsgBuf <- &kafkaMsg{key: key, value: value}:
		return nil
	default:
		// 缓冲满了，降级为同步发送（带限流）
		if producerLimiter != nil {
			limitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := producerLimiter.Wait(limitCtx); err != nil {
				return fmt.Errorf("rate limit wait timeout: %w", err)
			}
		}
		return kafkaWriter.WriteMessages(ctx, kafka.Message{
			Key:   key,
			Value: value,
		})
	}
}

// SendKafkaMsgSync 同步发送消息（用于需要确认的场景）
func SendKafkaMsgSync(ctx context.Context, key, value []byte) error {
	return kafkaWriter.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
	})
}

// CloseKafkaProducer 关闭 Kafka Producer
func CloseKafkaProducer() error {
	// 关闭发送协程
	if kafkaStopCh != nil {
		close(kafkaStopCh)
		kafkaWg.Wait()
	}

	// 关闭 Writer
	if kafkaWriter != nil {
		if err := kafkaWriter.Close(); err != nil {
			logger.Error.Printf("close kafka writer failed: %v", err)
			return err
		}
	}

	logger.Info.Println("Kafka producer closed")
	return nil
}

// GetKafkaBrokers 获取 Kafka Brokers（供 Consumer 使用）
func GetKafkaBrokers() []string {
	return kafkaBrokers
}

// GetKafkaTopic 获取 Kafka Topic
func GetKafkaTopic() string {
	return kafkaTopic
}
