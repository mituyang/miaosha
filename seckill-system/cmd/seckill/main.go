package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	pb "seckill-system/api/proto/seckill"
	grpcServer "seckill-system/internal/grpc"
	"seckill-system/pkg/config"
	"seckill-system/pkg/kafka"
	"seckill-system/pkg/redis"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化 Redis
	if err := redis.InitRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB); err != nil {
		log.Fatalf("初始化 Redis 失败: %v", err)
	}
	log.Println("Redis 连接成功")

	// 初始化 Kafka 生产者
	kafka.InitProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer kafka.Close()
	log.Println("Kafka 生产者初始化成功")

	// 创建 gRPC 服务器
	lis, err := net.Listen("tcp", ":"+cfg.GRPC.Port)
	if err != nil {
		log.Fatalf("监听端口失败: %v", err)
	}

	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(10*1024*1024),
		grpc.MaxSendMsgSize(10*1024*1024),
	)

	// 注册秒杀服务
	pb.RegisterSeckillServiceServer(s, grpcServer.NewSeckillServer())

	log.Printf("gRPC 秒杀服务启动在 :%s...", cfg.GRPC.Port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("启动 gRPC 服务失败: %v", err)
	}
}
