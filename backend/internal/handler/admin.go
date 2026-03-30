package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"seckill/internal/dto"
	"seckill/internal/service"
	"seckill/pkg/util"
)

type AdminHandler struct {
	adminSvc *service.AdminService
}

func NewAdminHandler(adminSvc *service.AdminService) *AdminHandler {
	return &AdminHandler{adminSvc: adminSvc}
}

// ListGoods 查询商品列表
func (h *AdminHandler) ListGoods(c *gin.Context) {
	status, ok := parseOptionalStatus(c)
	if !ok {
		return
	}

	page, pageSize, ok := parsePageQuery(c)
	if !ok {
		return
	}

	goods, total, err := h.adminSvc.ListGoods(c.Query("keyword"), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取商品列表失败"))
		return
	}
	c.JSON(http.StatusOK, util.Success(dto.PageResponse{
		Items:    goods,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}))
}

// CreateGoods 创建商品
func (h *AdminHandler) CreateGoods(c *gin.Context) {
	var req dto.AdminGoodsUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品参数无效"))
		return
	}

	goods, err := h.adminSvc.CreateGoods(c.Request.Context(), req)
	if err != nil {
		if err == service.ErrInvalidStatus {
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品状态无效"))
			return
		}
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "创建商品失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(goods))
}

// UpdateGoods 更新商品
func (h *AdminHandler) UpdateGoods(c *gin.Context) {
	goodsID, err := util.ParseUint64(c.Param("goods_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品ID无效"))
		return
	}

	var req dto.AdminGoodsUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品参数无效"))
		return
	}

	if err := h.adminSvc.UpdateGoods(c.Request.Context(), goodsID, req); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, util.Error(util.CodeParamError, "商品不存在"))
		case err == service.ErrInvalidStatus:
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品状态无效"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "更新商品失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(nil))
}

// DeleteGoods 删除商品
func (h *AdminHandler) DeleteGoods(c *gin.Context) {
	goodsID, err := util.ParseUint64(c.Param("goods_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品ID无效"))
		return
	}

	if err := h.adminSvc.DeleteGoods(c.Request.Context(), goodsID); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, util.Error(util.CodeParamError, "商品不存在"))
		case err == service.ErrGoodsHasOrders:
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品已有订单，不能删除"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "删除商品失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(nil))
}

// ListOrders 查询订单列表
func (h *AdminHandler) ListOrders(c *gin.Context) {
	status, ok := parseOptionalOrderStatus(c)
	if !ok {
		return
	}

	page, pageSize, ok := parsePageQuery(c)
	if !ok {
		return
	}

	orders, total, err := h.adminSvc.ListOrders(c.Query("keyword"), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取订单列表失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(dto.PageResponse{
		Items:    orders,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}))
}

// GetOrderDetail 查询订单详情
func (h *AdminHandler) GetOrderDetail(c *gin.Context) {
	orderID, err := util.ParseUint64(c.Param("order_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "订单ID无效"))
		return
	}

	order, err := h.adminSvc.GetOrderDetail(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, util.Error(util.CodeParamError, "订单不存在"))
			return
		}
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取订单详情失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(order))
}

// ListUsers 查询用户列表
func (h *AdminHandler) ListUsers(c *gin.Context) {
	status, ok := parseOptionalStatus(c)
	if !ok {
		return
	}

	page, pageSize, ok := parsePageQuery(c)
	if !ok {
		return
	}

	users, total, err := h.adminSvc.ListUsers(c.Query("keyword"), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取用户列表失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(dto.PageResponse{
		Items:    users,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}))
}

// UpdateUserStatus 更新用户状态
func (h *AdminHandler) UpdateUserStatus(c *gin.Context) {
	userID, err := util.ParseUint64(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "用户ID无效"))
		return
	}

	var req dto.AdminUserStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "用户状态参数无效"))
		return
	}

	if err := h.adminSvc.UpdateUserStatus(c.Request.Context(), userID, req.Status); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, util.Error(util.CodeParamError, "用户不存在"))
		case err == service.ErrInvalidStatus:
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "用户状态无效"))
		default:
			c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "更新用户状态失败"))
		}
		return
	}

	c.JSON(http.StatusOK, util.Success(nil))
}

// WarmUpGoods 预热单商品库存
func (h *AdminHandler) WarmUpGoods(c *gin.Context) {
	goodsID, err := util.ParseUint64(c.Param("goods_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品ID无效"))
		return
	}

	if err := h.adminSvc.WarmUpGoods(c.Request.Context(), goodsID); err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{"goods_id": goodsID}))
}

// WarmUpAll 全量预热库存
func (h *AdminHandler) WarmUpAll(c *gin.Context) {
	count, err := h.adminSvc.WarmUpAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, err.Error()))
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{"count": count}))
}

// GetStats 查询运营统计
func (h *AdminHandler) GetStats(c *gin.Context) {
	stats, err := h.adminSvc.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取统计数据失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(stats))
}

// RebuildStats 强制重建运营统计快照
func (h *AdminHandler) RebuildStats(c *gin.Context) {
	stats, err := h.adminSvc.RebuildStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "重建统计快照失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(stats))
}

// Ping 校验管理员凭证
func (h *AdminHandler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, util.Success(gin.H{"ok": h.adminSvc.Ping()}))
}

func parseOptionalStatus(c *gin.Context) (*uint8, bool) {
	raw := strings.TrimSpace(c.Query("status"))
	if raw == "" {
		return nil, true
	}

	value, err := strconv.ParseUint(raw, 10, 8)
	if err != nil || (value != 0 && value != 1) {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "状态参数无效"))
		return nil, false
	}

	status := uint8(value)
	return &status, true
}

func parseOptionalOrderStatus(c *gin.Context) (*uint8, bool) {
	raw := strings.TrimSpace(c.Query("status"))
	if raw == "" {
		return nil, true
	}

	value, err := strconv.ParseUint(raw, 10, 8)
	if err != nil || value > 2 {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "订单状态参数无效"))
		return nil, false
	}

	status := uint8(value)
	return &status, true
}

func parsePageQuery(c *gin.Context) (int, int, bool) {
	page := 1
	pageSize := 20

	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "分页参数无效"))
			return 0, 0, false
		}
		page = value
	}

	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > 100 {
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "分页大小无效"))
			return 0, 0, false
		}
		pageSize = value
	}

	return page, pageSize, true
}
