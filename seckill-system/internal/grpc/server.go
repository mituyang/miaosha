package grpc

import (
	"context"
	"log"

	pb "seckill-system/api/proto/seckill"
	"seckill-system/internal/service"
)

// SeckillServer gRPC 服务实现
type SeckillServer struct {
	pb.UnimplementedSeckillServiceServer
	seckillService *service.SeckillService
}

// NewSeckillServer 创建 gRPC 服务实例
// 异步模式：通过 Kafka 落库，不需要数据库连接
func NewSeckillServer() *SeckillServer {
	return &SeckillServer{
		seckillService: service.NewSeckillService(),
	}
}

// DoSeckill 实现秒杀 gRPC 接口
func (s *SeckillServer) DoSeckill(ctx context.Context, req *pb.SeckillRequest) (*pb.SeckillResponse, error) {
	log.Printf("收到秒杀请求: userID=%s, goodsID=%d", req.UserId, req.GoodsId)

	// 参数校验：用户名不能为空，商品ID必须大于0
	if req.UserId == "" || req.GoodsId <= 0 {
		return &pb.SeckillResponse{
			Code:    3,
			Message: "参数错误",
		}, nil
	}

	// 调用业务层执行秒杀
	result, err := s.seckillService.DoSeckill(ctx, req.UserId, req.GoodsId)
	if err != nil {
		log.Printf("秒杀处理异常: %v", err)
		// 不向客户端暴露内部错误细节
	}

	return &pb.SeckillResponse{
		Code:    result.Code,
		Message: result.Message,
		OrderId: result.OrderID,
	}, nil
}

// GetSeckillResult 查询秒杀结果（可扩展实现）
func (s *SeckillServer) GetSeckillResult(ctx context.Context, req *pb.ResultRequest) (*pb.ResultResponse, error) {
	// TODO: 实现查询逻辑，可以从 Redis 或数据库查询订单状态
	return &pb.ResultResponse{
		Status:  -1, // 排队中
		OrderId: "",
	}, nil
}
