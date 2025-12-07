package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"

	pb "seckill-system/api/proto/seckill"
	"seckill-system/internal/model"
	"seckill-system/pkg/kafka"
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
// 合并 Redis 缓存和 MySQL 中的订单，确保未落库的订单也能显示
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

	ctx := c.Request.Context()

	// 1. 从 Redis 缓存获取用户的订单ID列表
	cachedOrderIDs, _ := redis.GetUserOrderIDs(ctx, username)

	// 2. 查询 MySQL 中的订单
	var dbOrders []model.SeckillOrder
	dbQuery := h.db.Where("user_id = ?", username)
	if statusStr != "" && statusStr != "-1" {
		status, err := strconv.Atoi(statusStr)
		if err == nil && status >= 0 && status <= 2 {
			dbQuery = dbQuery.Where("status = ?", status)
		}
	}
	dbQuery.Order("created_at DESC").Find(&dbOrders)

	// 3. 构建已存在于 MySQL 的订单ID集合
	dbOrderIDSet := make(map[string]bool)
	for _, o := range dbOrders {
		dbOrderIDSet[o.OrderID] = true
	}

	// 4. 从 Redis 缓存获取未落库的订单详情
	var cachedOrders []OrderInfo
	for _, orderID := range cachedOrderIDs {
		// 跳过已在 MySQL 中的订单
		if dbOrderIDSet[orderID] {
			continue
		}
		// 从 Redis 获取订单详情
		orderCache, err := redis.GetOrderCache(ctx, orderID)
		if err != nil || orderCache == nil {
			continue
		}
		// 状态筛选
		if statusStr != "" && statusStr != "-1" {
			status, _ := strconv.Atoi(statusStr)
			if int(orderCache.Status) != status {
				continue
			}
		}
		cachedOrders = append(cachedOrders, OrderInfo{
			OrderID:   orderCache.OrderID,
			GoodsID:   orderCache.GoodsID,
			Status:    orderCache.Status,
			CreatedAt: time.UnixMilli(orderCache.CreatedAt).Format("2006-01-02 15:04:05"),
		})
	}

	// 5. 合并订单列表（Redis 缓存的放前面，因为是最新的）
	allOrders := make([]OrderInfo, 0, len(cachedOrders)+len(dbOrders))

	// 先添加 Redis 缓存中未落库的订单
	allOrders = append(allOrders, cachedOrders...)

	// 再添加 MySQL 中的订单
	for _, o := range dbOrders {
		allOrders = append(allOrders, OrderInfo{
			OrderID:   o.OrderID,
			GoodsID:   o.GoodsID,
			Status:    o.Status,
			CreatedAt: o.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	// 6. 获取商品ID列表并批量查询商品名称
	goodsIDSet := make(map[int64]bool)
	for _, o := range allOrders {
		goodsIDSet[o.GoodsID] = true
	}
	goodsIDs := make([]int64, 0, len(goodsIDSet))
	for id := range goodsIDSet {
		goodsIDs = append(goodsIDs, id)
	}

	goodsMap := make(map[int64]string)
	if len(goodsIDs) > 0 {
		var goods []model.SeckillGoods
		h.db.Where("id IN ?", goodsIDs).Find(&goods)
		for _, g := range goods {
			goodsMap[g.ID] = g.GoodsName
		}
	}

	// 7. 填充商品名称
	for i := range allOrders {
		allOrders[i].GoodsName = goodsMap[allOrders[i].GoodsID]
	}

	// 8. 按创建时间降序排序（最新的在最前面）
	sort.Slice(allOrders, func(i, j int) bool {
		return allOrders[i].CreatedAt > allOrders[j].CreatedAt
	})

	// 9. 分页处理
	total := int64(len(allOrders))
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)

	// 计算分页范围
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > int(total) {
		start = int(total)
	}
	if end > int(total) {
		end = int(total)
	}

	pagedOrders := allOrders[start:end]

	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: gin.H{
			"list":        pagedOrders,
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

	ctx := c.Request.Context()

	// 1. 先从 Redis 缓存查询订单
	orderCache, err := redis.GetOrderCache(ctx, req.OrderID)
	if err == nil && orderCache != nil && orderCache.UserID == username {
		// 缓存命中，基于缓存处理
		if orderCache.Status == 1 {
			c.JSON(http.StatusOK, Response{Code: 0, Message: "订单已支付"})
			return
		}
		if orderCache.Status == 2 {
			c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "订单已取消，无法支付"})
			return
		}

		// 检查是否超时
		createdAt := time.UnixMilli(orderCache.CreatedAt)
		if time.Since(createdAt) > time.Minute {
			// 超时，更新缓存状态并恢复库存
			if updateErr := redis.UpdateOrderStatus(ctx, req.OrderID, 2); updateErr != nil {
				fmt.Printf("更新订单缓存状态失败: %v\n", updateErr)
			}
			h.restoreStockOnly(ctx, orderCache.GoodsID, orderCache.UserID)
			c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "订单已超时取消"})
			return
		}

		// 更新 Redis 缓存状态为已支付
		if updateErr := redis.UpdateOrderStatus(ctx, req.OrderID, 1); updateErr != nil {
			fmt.Printf("更新订单缓存状态失败: %v\n", updateErr)
		}

		// 从延迟队列移除
		if removeErr := redis.RemoveFromDelayQueue(ctx, req.OrderID); removeErr != nil {
			fmt.Printf("移除延迟队列失败: %v\n", removeErr)
		}

		// 发送 Kafka 消息更新 MySQL
		if err := kafka.SendOrderStatusUpdate(ctx, req.OrderID, orderCache.UserID, orderCache.GoodsID, 1); err != nil {
			fmt.Printf("发送支付状态更新消息失败: %v\n", err)
		}

		c.JSON(http.StatusOK, Response{Code: 0, Message: "支付成功"})
		return
	}

	// 2. 缓存没有，查 MySQL
	var order model.SeckillOrder
	if err := h.db.Where("order_id = ? AND user_id = ?", req.OrderID, username).First(&order).Error; err != nil {
		c.JSON(http.StatusNotFound, Response{Code: 404, Message: "订单不存在"})
		return
	}

	if order.Status == 1 {
		c.JSON(http.StatusOK, Response{Code: 0, Message: "订单已支付"})
		return
	}
	if order.Status == 2 {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "订单已取消，无法支付"})
		return
	}

	if time.Since(order.CreatedAt) > time.Minute {
		h.cancelOrderAndRestoreStock(ctx, &order)
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "订单已超时取消"})
		return
	}

	// 更新 Redis 缓存状态
	if updateErr := redis.UpdateOrderStatus(ctx, req.OrderID, 1); updateErr != nil {
		fmt.Printf("更新订单缓存状态失败: %v\n", updateErr)
	}

	// 从延迟队列移除
	if removeErr := redis.RemoveFromDelayQueue(ctx, req.OrderID); removeErr != nil {
		fmt.Printf("移除延迟队列失败: %v\n", removeErr)
	}

	// 发送 Kafka 消息更新 MySQL
	if err := kafka.SendOrderStatusUpdate(ctx, req.OrderID, order.UserID, order.GoodsID, 1); err != nil {
		fmt.Printf("发送支付状态更新消息失败: %v\n", err)
	}

	c.JSON(http.StatusOK, Response{Code: 0, Message: "支付成功"})
}

// GetOrderDetail 获取订单详情（含剩余支付时间）
// 优先从 Redis 缓存读取，缓存没有再查 MySQL
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

	ctx := c.Request.Context()

	// 1. 先从 Redis 缓存查询
	orderCache, err := redis.GetOrderCache(ctx, orderID)
	if err == nil && orderCache != nil && orderCache.UserID == username {
		// 缓存命中，直接返回
		createdAt := time.UnixMilli(orderCache.CreatedAt)
		var remainSeconds int64 = 0
		if orderCache.Status == 0 {
			elapsed := time.Since(createdAt)
			remain := time.Minute - elapsed
			if remain > 0 {
				remainSeconds = int64(remain.Seconds())
			} else {
				// 已超时，更新缓存状态
				orderCache.Status = 2
				if updateErr := redis.UpdateOrderStatus(ctx, orderID, 2); updateErr != nil {
					fmt.Printf("更新订单缓存状态失败: %v\n", updateErr)
				}
			}
		}

		// 获取商品名称
		var goods model.SeckillGoods
		h.db.First(&goods, orderCache.GoodsID)

		c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "success",
			Data: gin.H{
				"order_id":       orderCache.OrderID,
				"goods_id":       orderCache.GoodsID,
				"goods_name":     goods.GoodsName,
				"status":         orderCache.Status,
				"created_at":     createdAt.Format("2006-01-02 15:04:05"),
				"remain_seconds": remainSeconds,
			},
		})
		return
	}

	// 2. 缓存没有，查 MySQL
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
			h.cancelOrderAndRestoreStock(ctx, &order)
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
// 优先从 Redis 缓存查询，缓存没有再查 MySQL
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

	ctx := c.Request.Context()

	// 1. 先从 Redis 缓存查询订单
	orderCache, err := redis.GetOrderCache(ctx, req.OrderID)
	if err == nil && orderCache != nil && orderCache.UserID == username {
		// 缓存命中，基于缓存处理
		if orderCache.Status != 0 {
			c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "订单状态不允许取消"})
			return
		}

		// 更新 Redis 缓存状态为已取消
		if updateErr := redis.UpdateOrderStatus(ctx, req.OrderID, 2); updateErr != nil {
			fmt.Printf("更新订单缓存状态失败: %v\n", updateErr)
		}

		// 恢复 Redis 库存
		h.restoreStockOnly(ctx, orderCache.GoodsID, orderCache.UserID)

		// 从延迟队列移除
		if removeErr := redis.RemoveFromDelayQueue(ctx, req.OrderID); removeErr != nil {
			fmt.Printf("移除延迟队列失败: %v\n", removeErr)
		}

		// 发送 Kafka 消息更新 MySQL（Consumer 会处理库存恢复）
		if err := kafka.SendOrderStatusUpdate(ctx, req.OrderID, orderCache.UserID, orderCache.GoodsID, 2); err != nil {
			fmt.Printf("发送取消状态更新消息失败: %v\n", err)
		}

		// 返回取消后的订单状态，方便前端更新 UI
		c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "订单已取消",
			Data: gin.H{
				"order_id": req.OrderID,
				"status":   2, // 已取消
			},
		})
		return
	}

	// 2. 缓存没有，查 MySQL
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

	// 更新 Redis 缓存状态为已取消
	if updateErr := redis.UpdateOrderStatus(ctx, req.OrderID, 2); updateErr != nil {
		fmt.Printf("更新订单缓存状态失败: %v\n", updateErr)
	}

	// 恢复 Redis 库存
	h.restoreStockOnly(ctx, order.GoodsID, order.UserID)

	// 从延迟队列移除
	if removeErr := redis.RemoveFromDelayQueue(ctx, req.OrderID); removeErr != nil {
		fmt.Printf("移除延迟队列失败: %v\n", removeErr)
	}

	// 发送 Kafka 消息更新 MySQL
	if err := kafka.SendOrderStatusUpdate(ctx, req.OrderID, order.UserID, order.GoodsID, 2); err != nil {
		fmt.Printf("发送取消状态更新消息失败: %v\n", err)
	}

	// 返回取消后的订单状态，方便前端更新 UI
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "订单已取消",
		Data: gin.H{
			"order_id": req.OrderID,
			"status":   2, // 已取消
		},
	})
}

// restoreStockOnly 仅恢复库存（用于 Redis 缓存订单超时，MySQL 可能还未落库）
// 不更新 MySQL 订单状态，只恢复 Redis 库存和已购记录
func (h *SeckillHandler) restoreStockOnly(ctx context.Context, goodsID int64, userID string) {
	stockKey := fmt.Sprintf("seckill:stock:%d", goodsID)
	boughtKey := fmt.Sprintf("seckill:bought:%d", goodsID)

	pipe := redis.Client.Pipeline()
	pipe.Incr(ctx, stockKey)          // 库存 +1
	pipe.SRem(ctx, boughtKey, userID) // 移除已购记录
	_, _ = pipe.Exec(ctx)
}

// cancelOrderAndRestoreStock 取消订单并恢复库存
// 更新 Redis 缓存状态，恢复 Redis 库存，发送 Kafka 消息更新 MySQL
func (h *SeckillHandler) cancelOrderAndRestoreStock(ctx context.Context, order *model.SeckillOrder) {
	// 更新 Redis 缓存状态为已取消
	if err := redis.UpdateOrderStatus(ctx, order.OrderID, 2); err != nil {
		fmt.Printf("更新订单缓存状态失败: orderID=%s, error=%v\n", order.OrderID, err)
	}

	// 恢复 Redis 库存
	h.restoreStockOnly(ctx, order.GoodsID, order.UserID)

	// 从延迟队列移除
	if err := redis.RemoveFromDelayQueue(ctx, order.OrderID); err != nil {
		fmt.Printf("移除延迟队列失败: orderID=%s, error=%v\n", order.OrderID, err)
	}

	// 发送 Kafka 消息更新 MySQL
	if err := kafka.SendOrderStatusUpdate(ctx, order.OrderID, order.UserID, order.GoodsID, 2); err != nil {
		fmt.Printf("发送取消状态更新消息失败: orderID=%s, error=%v\n", order.OrderID, err)
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
