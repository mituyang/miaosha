package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"seckill/internal/dto"
	"seckill/internal/service"
	"seckill/pkg/util"
)

type SeckillHandler struct {
	svc *service.SeckillService
}

func NewSeckillHandler(svc *service.SeckillService) *SeckillHandler {
	return &SeckillHandler{svc: svc}
}

// DoSeckill 秒杀接口
// POST /api/seckill/buy
func (h *SeckillHandler) DoSeckill(c *gin.Context) {
	// 记录用户请求时间
	requestTime := time.Now()

	var req dto.SeckillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "参数错误"))
		return
	}

	if req.ActivityID == 0 && req.GoodsID == 0 {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动ID或商品ID不能为空"))
		return
	}

	// 从 JWT 中间件获取 user_id
	userID := c.GetUint64("user_id")

	result, err := h.svc.DoSeckill(c.Request.Context(), userID, req.ActivityID, req.GoodsID, req.Quantity, requestTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "秒杀请求处理失败"))
		return
	}

	switch result {
	case service.ResultSuccess:
		c.JSON(http.StatusOK, util.Success(dto.SeckillResponse{Message: "秒杀请求已提交，请等待结果"}))
	case service.ResultSoldOut:
		c.JSON(http.StatusOK, util.Error(util.CodeSoldOut, "商品已售罄"))
	case service.ResultLimitExceed:
		c.JSON(http.StatusOK, util.Error(util.CodeLimitExceed, "超过限购数量"))
	case service.ResultNotOnSale:
		c.JSON(http.StatusOK, util.Error(util.CodeGoodsOffSale, "活动不可抢购"))
	default:
		c.JSON(http.StatusOK, util.Error(util.CodeServerError, "系统繁忙，请稍后重试"))
	}
}

// GetStock 查询库存接口
// GET /api/seckill/stock/:goods_id
func (h *SeckillHandler) GetStock(c *gin.Context) {
	goodsID, err := util.ParseUint64(c.Param("goods_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品ID无效"))
		return
	}

	stock, err := h.svc.GetStock(c.Request.Context(), goodsID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "查询库存失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{"goods_id": goodsID, "stock": stock}))
}

// WarmUp 库存预热接口 (单个商品)
// POST /api/seckill/warmup/:goods_id
func (h *SeckillHandler) WarmUp(c *gin.Context) {
	goodsID, err := util.ParseUint64(c.Param("goods_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "商品ID无效"))
		return
	}

	if err := h.svc.WarmUp(c.Request.Context(), goodsID); err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "库存预热失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{"goods_id": goodsID, "message": "库存预热成功"}))
}

// WarmUpAll 库存预热接口 (所有商品)
// POST /api/seckill/warmup
func (h *SeckillHandler) WarmUpAll(c *gin.Context) {
	count, err := h.svc.WarmUpAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "库存预热失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{"count": count, "message": "库存预热完成"}))
}
