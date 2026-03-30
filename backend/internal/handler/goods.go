package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"seckill/internal/service"
	"seckill/pkg/util"
)

type GoodsHandler struct {
	goodsSvc *service.GoodsService
}

func NewGoodsHandler(goodsSvc *service.GoodsService) *GoodsHandler {
	return &GoodsHandler{goodsSvc: goodsSvc}
}

// ListOnSaleGoods 获取上架商品列表
func (h *GoodsHandler) ListOnSaleGoods(c *gin.Context) {
	goods, err := h.goodsSvc.ListOnSaleGoods()
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取商品列表失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(goods))
}
