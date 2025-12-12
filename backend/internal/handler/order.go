package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"seckill/internal/service"
	"seckill/pkg/util"
)

type OrderHandler struct {
	orderSvc *service.OrderService
}

func NewOrderHandler(orderSvc *service.OrderService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc}
}

// GetOrders 获取当前用户订单列表
func (h *OrderHandler) GetOrders(c *gin.Context) {
	userID := c.GetUint64("user_id")

	orders, err := h.orderSvc.GetUserOrders(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取订单失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(orders))
}

// PayOrder 支付订单
func (h *OrderHandler) PayOrder(c *gin.Context) {
	userID := c.GetUint64("user_id")
	orderID, err := util.ParseUint64(c.Param("order_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "订单ID无效"))
		return
	}

	if err := h.orderSvc.PayOrder(c.Request.Context(), orderID, userID); err != nil {
		if err == service.ErrOrderStatusInvalid {
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "订单状态不允许支付"))
			return
		}
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "支付失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(nil))
}

// CancelOrder 取消订单
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	userID := c.GetUint64("user_id")
	orderID, err := util.ParseUint64(c.Param("order_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "订单ID无效"))
		return
	}

	if err := h.orderSvc.CancelOrder(c.Request.Context(), orderID, userID); err != nil {
		if err == service.ErrOrderStatusInvalid {
			c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "订单状态不允许取消"))
			return
		}
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "取消失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(nil))
}
