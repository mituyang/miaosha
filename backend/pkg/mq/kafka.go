package mq

import (
	"context"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"

	"seckill/internal/config"
	"seckill/pkg/logger"
)

var (
	kafkaWriter  *kafka.Writer
	kafkaMsgBuf  chan *kafkaMsg
	kafkaStopCh  chan struct{}
	kafkaWg      sync.WaitGroup
	kafkaTopic   string
	kafkaBrokers []string
	kafkaCfg     config.KafkaProducerConfig
)

type kafkaMsg struct {
	key   []byte
	value []byte
}

// InitKafkaProducer 初始化 Kafka Producer
func InitKafkaProducer(cfg *config.KafkaConfig) error {
	kafkaBrokers = cfg.Brokers
	kafkaTopic = cfg.Topic
	kafkaCfg = cfg.Producer

	// 应用默认值
	if kafkaCfg.BatchSize <= 0 {
		kafkaCfg.BatchSize = 1000
	}
	if kafkaCfg.BatchTimeoutMs <= 0 {
		kafkaCfg.BatchTimeoutMs = 5
	}
	if kafkaCfg.BufferSize <= 0 {
		kafkaCfg.BufferSize = 200000
	}
	if kafkaCfg.SenderCount <= 0 {
		kafkaCfg.SenderCount = 100
	}
	if kafkaCfg.MaxRetries <= 0 {
		kafkaCfg.MaxRetries = 3
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
	batch := make([]kafka.Message, 0, kafkaCfg.BatchSize)
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
			batch = append(batch, kafka.Message{
				Key:   msg.key,
				Value: msg.value,
			})
			if len(batch) >= kafkaCfg.BatchSize {
				sendKafkaBatch(batch)
				batch = make([]kafka.Message, 0, kafkaCfg.BatchSize)
				ticker.Reset(batchTimeout)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				sendKafkaBatch(batch)
				batch = make([]kafka.Message, 0, kafkaCfg.BatchSize)
			}
		}
	}
}

// sendKafkaBatch 发送一批消息
func sendKafkaBatch(msgs []kafka.Message) {
	if len(msgs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := kafkaWriter.WriteMessages(ctx, msgs...)
	if err != nil {
		logger.Error.Printf("kafka batch send failed: %v, count=%d", err, len(msgs))
		// 失败的消息重新入队
		for _, m := range msgs {
			select {
			case kafkaMsgBuf <- &kafkaMsg{key: m.Key, value: m.Value}:
			default:
				logger.Error.Printf("kafka buffer full, drop message")
			}
		}
	}
}

// SendKafkaMsg 发送消息到 Kafka（缓冲+批量发送）
func SendKafkaMsg(ctx context.Context, key, value []byte) error {
	select {
	case kafkaMsgBuf <- &kafkaMsg{key: key, value: value}:
		return nil
	default:
		// 缓冲满了，降级为同步发送
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
