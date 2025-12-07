package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"

	pb "seckill-system/api/proto/seckill"
	"seckill-system/internal/model"
	"seckill-system/pkg/redis"
)

// SeckillHandler HTTP 请求处理器
type SeckillHandler struct {
	grpcClient pb.SeckillServiceClient
	db         *gorm.DB
}

// NewSeckillHandler 创建 Handler 实例，连接 gRPC 服务
func NewSeckillHandler(grpcAddr string, db *gorm.DB) (*SeckillHandler, error) {
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
		db:         db,
	}, nil
}

// SeckillRequest 秒杀请求参数
type SeckillRequest struct {
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

	// 从 JWT Token 中获取用户名
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    401,
			Message: "请先登录",
		})
		return
	}

	// 设置超时上下文，防止请求堆积
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// 调用 gRPC 秒杀服务
	resp, err := h.grpcClient.DoSeckill(ctx, &pb.SeckillRequest{
		UserId:  username,
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
	username := c.GetString("username") // 从中间件获取用户名
	goodsID := c.GetInt64("goods_id")   // 从查询参数获取

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	resp, err := h.grpcClient.GetSeckillResult(ctx, &pb.ResultRequest{
		UserId:  username,
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

// GoodsInfo 商品信息
type GoodsInfo struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Stock int64  `json:"stock"`
}

// GetGoodsList 获取商品列表及实时库存
func (h *SeckillHandler) GetGoodsList(c *gin.Context) {
	ctx := c.Request.Context()

	// 从数据库获取商品列表
	var goods []model.SeckillGoods
	if err := h.db.Find(&goods).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "获取商品列表失败",
		})
		return
	}

	var goodsList []GoodsInfo
	for _, g := range goods {
		// 从 Redis 获取实时库存
		stockKey := fmt.Sprintf("seckill:stock:%d", g.ID)
		stock, err := redis.Client.Get(ctx, stockKey).Int64()
		if err != nil {
			stock = 0 // Redis 没有则默认为 0
		}

		goodsList = append(goodsList, GoodsInfo{
			ID:    g.ID,
			Name:  g.GoodsName,
			Stock: stock,
		})
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    goodsList,
	})
}

// SeckillRecord 秒杀记录（公开展示）
type SeckillRecord struct {
	GoodsName string `json:"goods_name"` // 商品名称
	Nickname  string `json:"nickname"`   // 用户昵称
	Username  string `json:"username"`   // 用户名（脱敏）
}

// GetSeckillRecords 获取所有秒杀成功记录（公开接口）
func (h *SeckillHandler) GetSeckillRecords(c *gin.Context) {
	// 查询所有已支付或待支付的订单，关联用户和商品信息
	var records []struct {
		GoodsName string `gorm:"column:goods_name"`
		Nickname  string `gorm:"column:nickname"`
		Username  string `gorm:"column:username"`
	}

	// 联表查询：订单 + 用户 + 商品
	err := h.db.Table("seckill_order AS o").
		Select("g.goods_name, u.nickname, u.username").
		Joins("LEFT JOIN seckill_goods AS g ON o.goods_id = g.id").
		Joins("LEFT JOIN users AS u ON o.user_id = u.username").
		Where("o.status = 1"). // 只显示已支付的订单
		Order("o.created_at DESC").
		Limit(50). // 最多显示 50 条
		Scan(&records).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "获取秒杀记录失败",
		})
		return
	}

	// 转换为响应格式，用户名脱敏
	var result []SeckillRecord
	for _, r := range records {
		result = append(result, SeckillRecord{
			GoodsName: r.GoodsName,
			Nickname:  r.Nickname,
			Username:  maskUsername(r.Username),
		})
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    result,
	})
}

// maskUsername 用户名脱敏，保留前2位和后1位
func maskUsername(username string) string {
	if len(username) <= 3 {
		return username
	}
	runes := []rune(username)
	if len(runes) <= 3 {
		return username
	}
	return string(runes[:2]) + "***" + string(runes[len(runes)-1:])
}

// ResetRequest 重置库存请求
type ResetRequest struct {
	GoodsID int64 `json:"goods_id" binding:"required,gt=0"`
	Stock   int64 `json:"stock" binding:"required,gt=0"`
}

// OrderInfo 订单信息（返回给前端）
type OrderInfo struct {
	OrderID   string `json:"order_id"`
	GoodsID   int64  `json:"goods_id"`
	GoodsName string `json:"goods_name"`
	Status    int8   `json:"status"` // 0-待支付, 1-已支付, 2-已取消
	CreatedAt string `json:"created_at"`
}

// GetMyOrders 获取当前用户的秒杀订单列表（支持筛选和分页）
func (h *SeckillHandler) GetMyOrders(c *gin.Context) {
	// 从 JWT Token 中获取用户名
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    401,
			Message: "请先登录",
		})
		return
	}

	// 获取查询参数
	statusStr := c.Query("status") // -1 或空表示全部，0-待支付，1-已支付，2-已取消
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "10")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}

	// 先处理超时订单（更新数据库状态）
	ctx := c.Request.Context()
	var pendingOrders []model.SeckillOrder
	h.db.Where("user_id = ? AND status = 0", username).Find(&pendingOrders)
	for i := range pendingOrders {
		if time.Since(pendingOrders[i].CreatedAt) > time.Minute {
			h.cancelOrderAndRestoreStock(ctx, &pendingOrders[i])
		}
	}

	// 构建查询条件
	query := h.db.Model(&model.SeckillOrder{}).Where("user_id = ?", username)
	if statusStr != "" && statusStr != "-1" {
		status, err := strconv.Atoi(statusStr)
		if err == nil && status >= 0 && status <= 2 {
			query = query.Where("status = ?", status)
		}
	}

	// 查询总数
	var total int64
	query.Count(&total)

	// 分页查询订单
	var orders []model.SeckillOrder
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "查询订单失败",
		})
		return
	}

	// 获取商品ID列表
	goodsIDs := make([]int64, 0, len(orders))
	for _, o := range orders {
		goodsIDs = append(goodsIDs, o.GoodsID)
	}

	// 批量查询商品信息
	goodsMap := make(map[int64]string)
	if len(goodsIDs) > 0 {
		var goods []model.SeckillGoods
		h.db.Where("id IN ?", goodsIDs).Find(&goods)
		for _, g := range goods {
			goodsMap[g.ID] = g.GoodsName
		}
	}

	// 组装返回数据
	orderList := make([]OrderInfo, 0, len(orders))
	for _, o := range orders {
		orderList = append(orderList, OrderInfo{
			OrderID:   o.OrderID,
			GoodsID:   o.GoodsID,
			GoodsName: goodsMap[o.GoodsID],
			Status:    o.Status,
			CreatedAt: o.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 计算总页数
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"list":        orderList,
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": totalPages,
		},
	})
}

// PayRequest 支付请求
type PayRequest struct {
	OrderID string `json:"order_id" binding:"required"`
}

// PayOrder 支付订单（模拟）
func (h *SeckillHandler) PayOrder(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    401,
			Message: "请先登录",
		})
		return
	}

	var req PayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	// 查询订单
	var order model.SeckillOrder
	if err := h.db.Where("order_id = ? AND user_id = ?", req.OrderID, username).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    404,
			Message: "订单不存在",
		})
		return
	}

	// 检查订单状态
	if order.Status == 1 {
		c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "订单已支付",
		})
		return
	}
	if order.Status == 2 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "订单已取消，无法支付",
		})
		return
	}

	// 检查是否超时（1分钟）
	if time.Since(order.CreatedAt) > time.Minute {
		// 超时自动取消
		h.cancelOrderAndRestoreStock(c.Request.Context(), &order)
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "订单已超时取消",
		})
		return
	}

	// 更新订单状态为已支付
	if err := h.db.Model(&order).Update("status", 1).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "支付失败",
		})
		return
	}

	// 支付成功，从延迟队列移除
	if err := redis.RemoveFromDelayQueue(c.Request.Context(), req.OrderID); err != nil {
		// 移除失败不影响主流程，只记录日志
		fmt.Printf("移除延迟队列失败: orderID=%s, error=%v\n", req.OrderID, err)
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "支付成功",
	})
}

// GetOrderDetail 获取订单详情（含剩余支付时间）
func (h *SeckillHandler) GetOrderDetail(c *gin.Context) {
	username := c.GetString("username")
	orderID := c.Query("order_id")

	if orderID == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "缺少订单号",
		})
		return
	}

	var order model.SeckillOrder
	if err := h.db.Where("order_id = ? AND user_id = ?", orderID, username).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    404,
			Message: "订单不存在",
		})
		return
	}

	// 获取商品名称
	var goods model.SeckillGoods
	h.db.First(&goods, order.GoodsID)

	// 计算剩余支付时间（秒）
	var remainSeconds int64 = 0
	if order.Status == 0 {
		elapsed := time.Since(order.CreatedAt)
		remain := time.Minute - elapsed
		if remain > 0 {
			remainSeconds = int64(remain.Seconds())
		} else {
			// 已超时，自动取消
			h.cancelOrderAndRestoreStock(c.Request.Context(), &order)
			order.Status = 2
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"order_id":       order.OrderID,
			"goods_id":       order.GoodsID,
			"goods_name":     goods.GoodsName,
			"status":         order.Status,
			"created_at":     order.CreatedAt.Format("2006-01-02 15:04:05"),
			"remain_seconds": remainSeconds,
		},
	})
}

// CancelOrder 用户主动取消订单
func (h *SeckillHandler) CancelOrder(c *gin.Context) {
	username := c.GetString("username")
	if username == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    401,
			Message: "请先登录",
		})
		return
	}

	var req PayRequest // 复用 PayRequest 结构
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	// 查询订单
	var order model.SeckillOrder
	if err := h.db.Where("order_id = ? AND user_id = ?", req.OrderID, username).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    404,
			Message: "订单不存在",
		})
		return
	}

	// 只有待支付的订单可以取消
	if order.Status != 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "订单状态不允许取消",
		})
		return
	}

	// 取消订单并恢复库存
	h.cancelOrderAndRestoreStock(c.Request.Context(), &order)

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "订单已取消",
	})
}

// cancelOrderAndRestoreStock 取消订单并恢复库存
// 使用条件更新防止重复取消导致库存多次恢复
func (h *SeckillHandler) cancelOrderAndRestoreStock(ctx context.Context, order *model.SeckillOrder) {
	// 只有状态为 0（待支付）时才更新为已取消，防止重复操作
	result := h.db.Model(order).Where("status = ?", 0).Update("status", 2)
	if result.Error != nil || result.RowsAffected == 0 {
		// 更新失败或订单已不是待支付状态，不恢复库存
		return
	}

	// 恢复 MySQL 库存
	h.db.Model(&model.SeckillGoods{}).Where("id = ?", order.GoodsID).
		Update("stock", gorm.Expr("stock + 1"))

	// 恢复 Redis 库存
	stockKey := fmt.Sprintf("seckill:stock:%d", order.GoodsID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", order.GoodsID)

	pipe := redis.Client.Pipeline()
	pipe.Incr(ctx, stockKey)                // 库存 +1
	pipe.SRem(ctx, boughtKey, order.UserID) // 移除已购记录
	_, _ = pipe.Exec(ctx)

	// 从延迟队列移除
	if err := redis.RemoveFromDelayQueue(ctx, order.OrderID); err != nil {
		fmt.Printf("移除延迟队列失败: orderID=%s, error=%v\n", order.OrderID, err)
	}
}

// ResetStock 重置商品库存（同时清除已购用户记录）
// 用于管理员手动删除订单后恢复库存
func (h *SeckillHandler) ResetStock(c *gin.Context) {
	var req ResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    400,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	stockKey := fmt.Sprintf("seckill:stock:%d", req.GoodsID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", req.GoodsID)

	// 使用 Pipeline 原子执行：重置库存 + 清空已购用户
	pipe := redis.Client.Pipeline()
	pipe.Set(ctx, stockKey, req.Stock, 0)
	pipe.Del(ctx, boughtKey)
	_, err := pipe.Exec(ctx)

	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    500,
			Message: "重置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: fmt.Sprintf("商品 %d 库存已重置为 %d，已购记录已清空", req.GoodsID, req.Stock),
	})
}
