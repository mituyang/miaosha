package mq

import (
	"context"
	"fmt"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"

	"seckill/internal/config"
)

var Producer rocketmq.Producer

func InitProducer(cfg *config.RocketMQConfig) error {
	p, err := rocketmq.NewProducer(
		producer.WithNameServer([]string{cfg.NameSrv}),
		producer.WithRetry(2),
		producer.WithGroupName("seckill_producer_group"),
	)
	if err != nil {
		return fmt.Errorf("failed to create producer: %w", err)
	}

	if err := p.Start(); err != nil {
		return fmt.Errorf("failed to start producer: %w", err)
	}

	Producer = p
	return nil
}

// SendSeckillMsg 发送秒杀消息
func SendSeckillMsg(ctx context.Context, topic string, body []byte) error {
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}

	_, err := Producer.SendSync(ctx, msg)
	return err
}

// SendDelayMsg 发送延迟消息
// delayLevel: 1=1s, 2=5s, 3=10s, 4=30s, 5=1m, 6=2m, 7=3m, 8=4m, 9=5m, 10=6m, 11=7m, 12=8m, 13=9m, 14=10m, 15=20m, 16=30m, 17=1h, 18=2h
func SendDelayMsg(ctx context.Context, topic string, body []byte, delayLevel int) error {
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	msg.WithDelayTimeLevel(delayLevel)

	_, err := Producer.SendSync(ctx, msg)
	return err
}

func CloseProducer() error {
	if Producer != nil {
		return Producer.Shutdown()
	}
	return nil
}
