package mq

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"

	"seckill/internal/config"
	"seckill/pkg/logger"
)

const (
	delayMsgMaxRetries   = 5
	delayMsgBaseDelay    = 100 * time.Millisecond // 初始重试间隔
	delayMsgMaxDelay     = 2 * time.Second        // 最大重试间隔
	delayMsgBackoffRatio = 2                      // 退避倍数

	// 批量发送配置
	batchSize    = 200                  // 每批最大消息数
	batchTimeout = 5 * time.Millisecond // 批量等待超时
	bufferSize   = 100000               // 缓冲队列大小
	senderCount  = 10                   // 发送协程数
)

var (
	Producer     rocketmq.Producer
	msgBuffer    chan *bufferedMsg
	stopCh       chan struct{}
	wg           sync.WaitGroup
	seckillTopic string
)

type bufferedMsg struct {
	body []byte
}

func InitProducer(cfg *config.RocketMQConfig) error {
	p, err := rocketmq.NewProducer(
		producer.WithNameServer([]string{cfg.NameSrv}),
		producer.WithRetry(2),
		producer.WithGroupName("seckill_producer_group"),
		producer.WithCreateTopicKey("TBW102"),
	)
	if err != nil {
		return fmt.Errorf("failed to create producer: %w", err)
	}

	if err := p.Start(); err != nil {
		return fmt.Errorf("failed to start producer: %w", err)
	}

	Producer = p
	seckillTopic = cfg.Topic

	// 初始化缓冲队列和发送协程
	msgBuffer = make(chan *bufferedMsg, bufferSize)
	stopCh = make(chan struct{})

	for i := 0; i < senderCount; i++ {
		wg.Add(1)
		go batchSender(i)
	}

	return nil
}

// batchSender 批量发送协程
func batchSender(_ int) {
	defer wg.Done()

	batch := make([]*primitive.Message, 0, batchSize)
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			// 关闭前发送剩余消息
			if len(batch) > 0 {
				sendBatch(batch)
			}
			return

		case msg := <-msgBuffer:
			batch = append(batch, &primitive.Message{
				Topic: seckillTopic,
				Body:  msg.body,
			})
			if len(batch) >= batchSize {
				sendBatch(batch)
				batch = make([]*primitive.Message, 0, batchSize)
				ticker.Reset(batchTimeout)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				sendBatch(batch)
				batch = make([]*primitive.Message, 0, batchSize)
			}
		}
	}
}

// sendBatch 发送一批消息
func sendBatch(msgs []*primitive.Message) {
	if len(msgs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Producer.SendSync(ctx, msgs...)
	if err != nil {
		logger.Error.Printf("batch send failed: %v, count=%d", err, len(msgs))
		// 失败的消息重新入队
		for _, m := range msgs {
			select {
			case msgBuffer <- &bufferedMsg{body: m.Body}:
			default:
				logger.Error.Printf("buffer full, drop message")
			}
		}
	}
}

// EnsureTopic 确保 topic 存在，通过发送一条空消息触发自动创建
func EnsureTopic(ctx context.Context, topic string) error {
	msg := &primitive.Message{
		Topic: topic,
		Body:  []byte("init"),
	}
	_, err := Producer.SendSync(ctx, msg)
	return err
}

// SendSeckillMsg 发送秒杀消息（缓冲+批量发送，高吞吐）
func SendSeckillMsg(ctx context.Context, topic string, body []byte) error {
	select {
	case msgBuffer <- &bufferedMsg{body: body}:
		return nil
	default:
		// 缓冲满了，降级为同步发送
		msg := &primitive.Message{
			Topic: topic,
			Body:  body,
		}
		_, err := Producer.SendSync(ctx, msg)
		return err
	}
}

// SendDelayMsg 发送延迟消息（带指数退避重试）
// delayLevel: 1=1s, 2=5s, 3=10s, 4=30s, 5=1m, 6=2m, 7=3m, 8=4m, 9=5m, 10=6m, 11=7m, 12=8m, 13=9m, 14=10m, 15=20m, 16=30m, 17=1h, 18=2h
func SendDelayMsg(ctx context.Context, topic string, body []byte, delayLevel int) error {
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	msg.WithDelayTimeLevel(delayLevel)

	var lastErr error
	delay := delayMsgBaseDelay

	for range delayMsgMaxRetries {
		_, err := Producer.SendSync(ctx, msg)
		if err == nil {
			return nil
		}
		lastErr = err
		// 只对 broker busy 错误重试
		if !isBrokerBusyError(err) {
			return err
		}
		// 指数退避：100ms -> 200ms -> 400ms -> 800ms -> 1600ms
		time.Sleep(delay)
		delay *= delayMsgBackoffRatio
		if delay > delayMsgMaxDelay {
			delay = delayMsgMaxDelay
		}
		// 检查 context 是否已取消
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return lastErr
}

// SendTimerMsg 发送定时消息（指定精确投递时间，RocketMQ 5.x）
// deliverTime: 消息投递的精确时间
func SendTimerMsg(ctx context.Context, topic string, body []byte, deliverTime time.Time) error {
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	// RocketMQ 5.x 定时消息：使用 TIMER_DELIVER_MS 设置投递时间戳（毫秒）
	msg.WithProperty("TIMER_DELIVER_MS", fmt.Sprintf("%d", deliverTime.UnixMilli()))

	var lastErr error
	delay := delayMsgBaseDelay

	for range delayMsgMaxRetries {
		_, err := Producer.SendSync(ctx, msg)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isBrokerBusyError(err) {
			return err
		}
		time.Sleep(delay)
		delay *= delayMsgBackoffRatio
		if delay > delayMsgMaxDelay {
			delay = delayMsgMaxDelay
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return lastErr
}

// isBrokerBusyError 判断是否是 broker 繁忙错误
func isBrokerBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "broker busy") ||
		strings.Contains(errStr, "flow control") ||
		strings.Contains(errStr, "TIMEOUT_CLEAN_QUEUE")
}

func CloseProducer() error {
	// 关闭发送协程
	if stopCh != nil {
		close(stopCh)
		wg.Wait()
	}

	if Producer != nil {
		return Producer.Shutdown()
	}
	return nil
}
