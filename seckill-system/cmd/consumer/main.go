package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"seckill-system/internal/model"
	"seckill-system/pkg/config"
	"seckill-system/pkg/kafka"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化数据库连接
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	log.Println("数据库连接成功")

	// 自动迁移表结构
	if err := db.AutoMigrate(&model.SeckillGoods{}, &model.SeckillOrder{}); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	// 创建 Kafka 消费者
	consumer := kafka.NewConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.Topic,
		cfg.Kafka.GroupID,
		db,
	)
	defer consumer.Close()

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 监听系统信号，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("收到退出信号，正在关闭...")
		cancel()
	}()

	// 启动消费者
	consumer.Start(ctx)
}
