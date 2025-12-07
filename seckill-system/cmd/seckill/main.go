package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

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

	// 初始化 MySQL 数据库（同步写入订单需要）
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("初始化 MySQL 失败: %v", err)
	}
	log.Println("MySQL 连接成功")

	// 从 MySQL 预热库存到 Redis
	preloadStockFromMySQL(db)

	// 初始化 Kafka 生产者（异步落库）
	kafka.InitProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer kafka.Close()
	log.Println("Kafka 生产者初始化成功")

	// 创建 gRPC 服务器
	// 高并发优化：增大并发流数量和窗口大小
	lis, err := net.Listen("tcp", ":"+cfg.GRPC.Port)
	if err != nil {
		log.Fatalf("监听端口失败: %v", err)
	}

	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(10*1024*1024),
		grpc.MaxSendMsgSize(10*1024*1024),
		grpc.MaxConcurrentStreams(10000),   // 最大并发流数
		grpc.InitialWindowSize(1<<20),      // 1MB 初始窗口
		grpc.InitialConnWindowSize(1<<20),  // 1MB 连接窗口
		grpc.NumStreamWorkers(uint32(100)), // 流处理工作协程数
	)

	// 注册秒杀服务（异步模式，通过 Kafka 落库）
	pb.RegisterSeckillServiceServer(s, grpcServer.NewSeckillServer())

	log.Printf("gRPC 秒杀服务启动在 :%s（异步模式）...", cfg.GRPC.Port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("启动 gRPC 服务失败: %v", err)
	}
}

// preloadStockFromMySQL 从 MySQL 预热库存到 Redis
// 服务启动时调用，确保 Redis 库存与 MySQL 一致
func preloadStockFromMySQL(db *gorm.DB) {
	type SeckillGoods struct {
		ID    int64
		Stock int
	}

	var goods []SeckillGoods
	if err := db.Table("seckill_goods").Find(&goods).Error; err != nil {
		log.Printf("预热库存失败: %v", err)
		return
	}

	ctx := context.Background()
	for _, g := range goods {
		if err := redis.PreloadStock(ctx, g.ID, g.Stock); err != nil {
			log.Printf("预热商品 %d 库存失败: %v", g.ID, err)
		} else {
			log.Printf("预热商品 %d 库存: %d", g.ID, g.Stock)
		}

		// 预热已购用户集合
		preloadBoughtUsers(db, ctx, g.ID)
	}
	log.Println("库存预热完成")
}

// preloadBoughtUsers 从 MySQL 订单表预热已购用户到 Redis
func preloadBoughtUsers(db *gorm.DB, ctx context.Context, goodsID int64) {
	// 查询该商品的有效订单（待支付或已支付）
	type Order struct {
		UserID string
	}
	var orders []Order
	if err := db.Table("seckill_order").
		Select("user_id").
		Where("goods_id = ? AND status IN (0, 1)", goodsID).
		Find(&orders).Error; err != nil {
		log.Printf("预热商品 %d 已购用户失败: %v", goodsID, err)
		return
	}

	if len(orders) == 0 {
		// 清空 Redis 中的已购记录（确保一致性）
		boughtKey := fmt.Sprintf("seckill:bought:%d", goodsID)
		redis.Client.Del(ctx, boughtKey)
		log.Printf("商品 %d 无有效订单，已清空已购记录", goodsID)
		return
	}

	// 批量添加到 Redis Set
	boughtKey := fmt.Sprintf("seckill:bought:%d", goodsID)
	// 先清空再添加，确保一致性
	redis.Client.Del(ctx, boughtKey)

	userIDs := make([]interface{}, len(orders))
	for i, o := range orders {
		userIDs[i] = o.UserID
	}
	if err := redis.Client.SAdd(ctx, boughtKey, userIDs...).Err(); err != nil {
		log.Printf("预热商品 %d 已购用户失败: %v", goodsID, err)
	} else {
		log.Printf("预热商品 %d 已购用户: %d 人", goodsID, len(orders))
	}
}
