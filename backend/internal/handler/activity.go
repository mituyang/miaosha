package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"seckill/internal/dto"
	"seckill/internal/service"
	"seckill/pkg/util"
)

type ActivityHandler struct {
	activitySvc *service.ActivityService
	seckillSvc  *service.SeckillService
}

func NewActivityHandler(activitySvc *service.ActivityService, seckillSvc *service.SeckillService) *ActivityHandler {
	return &ActivityHandler{activitySvc: activitySvc, seckillSvc: seckillSvc}
}

// ListPublicActivities 查询公开活动列表
func (h *ActivityHandler) ListPublicActivities(c *gin.Context) {
	activities, err := h.activitySvc.ListPublicActivities(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取活动列表失败"))
		return
	}
	c.JSON(http.StatusOK, util.Success(activities))
}

// GetActivityStock 查询活动库存
func (h *ActivityHandler) GetActivityStock(c *gin.Context) {
	activityID, err := util.ParseUint64(c.Param("activity_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动ID无效"))
		return
	}

	stock, err := h.seckillSvc.GetActivityStock(c.Request.Context(), activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "查询库存失败"))
		return
	}

	c.JSON(http.StatusOK, util.Success(gin.H{"activity_id": activityID, "stock": stock}))
}

// ListActivities 查询后台活动列表
func (h *ActivityHandler) ListActivities(c *gin.Context) {
	status, ok := parseOptionalActivityStatus(c)
	if !ok {
		return
	}
	page, pageSize, ok := parsePageQuery(c)
	if !ok {
		return
	}

	activities, total, err := h.activitySvc.ListAdminActivities(c.Query("keyword"), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, "获取活动列表失败"))
		return
	}
	c.JSON(http.StatusOK, util.Success(dto.PageResponse{
		Items:    activities,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}))
}

// CreateActivity 创建活动
func (h *ActivityHandler) CreateActivity(c *gin.Context) {
	var req dto.AdminActivityUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动参数无效"))
		return
	}

	activity, err := h.activitySvc.CreateActivity(c.Request.Context(), req)
	if err != nil {
		h.writeActivityError(c, err, "创建活动失败")
		return
	}
	c.JSON(http.StatusOK, util.Success(activity))
}

// UpdateActivity 更新活动
func (h *ActivityHandler) UpdateActivity(c *gin.Context) {
	activityID, err := util.ParseUint64(c.Param("activity_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动ID无效"))
		return
	}

	var req dto.AdminActivityUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动参数无效"))
		return
	}

	if err := h.activitySvc.UpdateActivity(c.Request.Context(), activityID, req); err != nil {
		h.writeActivityError(c, err, "更新活动失败")
		return
	}
	c.JSON(http.StatusOK, util.Success(nil))
}

// WarmUpActivity 预热活动
func (h *ActivityHandler) WarmUpActivity(c *gin.Context) {
	activityID, err := util.ParseUint64(c.Param("activity_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动ID无效"))
		return
	}
	if err := h.activitySvc.WarmUpActivity(c.Request.Context(), activityID); err != nil {
		h.writeActivityError(c, err, "活动预热失败")
		return
	}
	c.JSON(http.StatusOK, util.Success(gin.H{"activity_id": activityID}))
}

// UpdateActivityStatus 更新活动状态
func (h *ActivityHandler) UpdateActivityStatus(c *gin.Context) {
	activityID, err := util.ParseUint64(c.Param("activity_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动ID无效"))
		return
	}

	var req dto.AdminActivityStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动状态参数无效"))
		return
	}

	if err := h.activitySvc.UpdateActivityStatus(c.Request.Context(), activityID, req.Status); err != nil {
		h.writeActivityError(c, err, "更新活动状态失败")
		return
	}
	c.JSON(http.StatusOK, util.Success(nil))
}

func (h *ActivityHandler) writeActivityError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, util.Error(util.CodeParamError, "活动或商品不存在"))
	case errors.Is(err, service.ErrActivityTimeInvalid):
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动时间无效"))
	case errors.Is(err, service.ErrActivityOverlap):
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "同一商品启用活动时间不能重叠"))
	case errors.Is(err, service.ErrActivityStatus):
		c.JSON(http.StatusBadRequest, util.Error(util.CodeParamError, "活动状态无效"))
	default:
		c.JSON(http.StatusInternalServerError, util.Error(util.CodeServerError, fallback))
	}
}
