package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "seckill-system/api/proto/seckill"
)

// SeckillHandler HTTP 请求处理器
type SeckillHandler struct {
	grpcClient pb.SeckillServiceClient
}

// NewSeckillHandler 创建 Handler 实例，连接 gRPC 服务
func NewSeckillHandler(grpcAddr string) (*SeckillHandler, error) {
	// 建立 gRPC 连接
	conn, err := grpc.Dial(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(10*1024*1024)),
	)
	if err != nil {
		return nil, err
	}

	return &SeckillHandler{
		grpcClient: pb.NewSeckillServiceClient(conn),
	}, nil
}

// SeckillRequest 秒杀请求参数
type SeckillRequest struct {
	UserID  int64 `json:"user_id" binding:"required,gt=0"`
	GoodsID int64 `json:"goods_id" binding:"required,gt=0"`
}

// Response 统一响应结构
type Response struct {
	Code    int32       `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// DoSeckill 处理秒杀 HTTP 请求
func (h *SeckillHandler) DoSeckill(c *gin.Context) {
	var req SeckillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	// 设置超时上下文，防止请求堆积
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// 调用 gRPC 秒杀服务
	resp, err := h.grpcClient.DoSeckill(ctx, &pb.SeckillRequest{
		UserId:  req.UserID,
		GoodsId: req.GoodsID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "服务暂时不可用",
		})
		return
	}

	// 根据业务状态码返回对应 HTTP 状态
	httpStatus := http.StatusOK
	if resp.Code != 0 {
		httpStatus = http.StatusOK // 业务失败也返回 200，通过 code 区分
	}

	c.JSON(httpStatus, Response{
		Code:    resp.Code,
		Message: resp.Message,
		Data: gin.H{
			"order_id": resp.OrderId,
		},
	})
}

// GetResult 查询秒杀结果
func (h *SeckillHandler) GetResult(c *gin.Context) {
	userID := c.GetInt64("user_id")   // 从中间件获取
	goodsID := c.GetInt64("goods_id") // 从查询参数获取

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	resp, err := h.grpcClient.GetSeckillResult(ctx, &pb.ResultRequest{
		UserId:  userID,
		GoodsId: goodsID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "查询失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"status":   resp.Status,
			"order_id": resp.OrderId,
		},
	})
}
