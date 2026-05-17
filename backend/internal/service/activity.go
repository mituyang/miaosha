package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"seckill/internal/config"
	"seckill/internal/dto"
	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/redis"
)

var (
	ErrActivityTimeInvalid = errors.New("activity time invalid")
	ErrActivityOverlap     = errors.New("activity time overlap")
	ErrActivityStatus      = errors.New("activity status invalid")
	ErrActivityConfig      = errors.New("activity config invalid")
)

type ActivityService struct {
	cfg          *config.Config
	activityRepo *repository.SeckillActivityRepository
	goodsRepo    *repository.GoodsRepository
	seckillSvc   *SeckillService
}

func NewActivityService(cfg *config.Config, seckillSvc *SeckillService) *ActivityService {
	return &ActivityService{
		cfg:          cfg,
		activityRepo: repository.NewSeckillActivityRepository(database.DB),
		goodsRepo:    repository.NewGoodsRepository(database.DB),
		seckillSvc:   seckillSvc,
	}
}

// EnsureDefaultActivities 为已有商品补默认活动
func EnsureDefaultActivities(cfg *config.Config) error {
	svc := NewActivityService(cfg, nil)
	goods, err := svc.goodsRepo.GetAll()
	if err != nil {
		return err
	}
	for i := range goods {
		if err := svc.EnsureDefaultForGoods(context.Background(), &goods[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *ActivityService) EnsureDefaultForGoods(ctx context.Context, goods *model.Goods) error {
	activity, err := s.activityRepo.FindDefaultByGoodsID(goods.ID)
	if err == nil {
		if redis.Client != nil {
			_ = redis.SetDefaultActivityID(ctx, goods.ID, activity.ID)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	startTime, endTime, err := s.defaultActivityWindow(time.Now())
	if err != nil {
		return err
	}
	title, err := s.defaultActivityTitle(goods.ProductName)
	if err != nil {
		return err
	}
	if s.cfg.Seckill.MaxBuyLimit <= 0 {
		return fmt.Errorf("%w: seckill.max_buy_limit must be greater than 0", ErrActivityConfig)
	}

	status := model.SeckillActivityStatusDisabled
	if s.cfg.Seckill.DefaultActivity.Enabled {
		status = model.SeckillActivityStatusRunning
	}

	activity = &model.SeckillActivity{
		GoodsID:      goods.ID,
		Title:        title,
		StartTime:    startTime,
		EndTime:      endTime,
		Status:       status,
		MaxBuyLimit:  uint(s.cfg.Seckill.MaxBuyLimit),
		WarmupStatus: model.SeckillActivityWarmupPending,
		IsDefault:    1,
	}
	if err := s.activityRepo.Create(activity); err != nil {
		return err
	}
	if redis.Client != nil {
		_ = redis.SetDefaultActivityID(ctx, goods.ID, activity.ID)
	}
	return nil
}

func (s *ActivityService) ListPublicActivities(ctx context.Context) ([]dto.ActivityResponse, error) {
	_ = ctx
	activities, err := s.activityRepo.ListPublic(time.Now())
	if err != nil {
		return nil, err
	}
	return mapActivityResponses(activities), nil
}

func (s *ActivityService) ListAdminActivities(keyword string, status *uint8, page, pageSize int) ([]dto.ActivityResponse, int64, error) {
	activities, total, err := s.activityRepo.ListPage(repository.SeckillActivityFilter{
		Keyword: strings.TrimSpace(keyword),
		Status:  status,
	}, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return mapActivityResponses(activities), total, nil
}

func (s *ActivityService) CreateActivity(ctx context.Context, req dto.AdminActivityUpsertRequest) (*dto.ActivityResponse, error) {
	_ = ctx
	startTime, endTime, err := parseActivityWindow(req.StartTime, req.EndTime)
	if err != nil {
		return nil, err
	}
	if err := validateActivityStatus(req.Status); err != nil {
		return nil, err
	}
	if _, err := s.goodsRepo.GetByID(req.GoodsID); err != nil {
		return nil, err
	}
	if err := s.ensureNoOverlap(req.GoodsID, 0, startTime, endTime, req.Status); err != nil {
		return nil, err
	}

	activity := &model.SeckillActivity{
		GoodsID:      req.GoodsID,
		Title:        strings.TrimSpace(req.Title),
		StartTime:    startTime,
		EndTime:      endTime,
		Status:       req.Status,
		MaxBuyLimit:  req.MaxBuyLimit,
		WarmupStatus: model.SeckillActivityWarmupPending,
		IsDefault:    0,
	}
	if err := s.activityRepo.Create(activity); err != nil {
		return nil, err
	}

	return s.GetActivityResponse(activity.ID)
}

func (s *ActivityService) UpdateActivity(ctx context.Context, id uint64, req dto.AdminActivityUpsertRequest) error {
	current, err := s.activityRepo.GetByID(id)
	if err != nil {
		return err
	}
	startTime, endTime, err := parseActivityWindow(req.StartTime, req.EndTime)
	if err != nil {
		return err
	}
	if err := validateActivityStatus(req.Status); err != nil {
		return err
	}
	if _, err := s.goodsRepo.GetByID(req.GoodsID); err != nil {
		return err
	}
	if err := s.ensureNoOverlap(req.GoodsID, id, startTime, endTime, req.Status); err != nil {
		return err
	}

	current.GoodsID = req.GoodsID
	current.Title = strings.TrimSpace(req.Title)
	current.StartTime = startTime
	current.EndTime = endTime
	current.Status = req.Status
	current.MaxBuyLimit = req.MaxBuyLimit
	current.WarmupStatus = model.SeckillActivityWarmupPending
	if err := s.activityRepo.Update(current); err != nil {
		return err
	}
	if current.IsDefault == 1 {
		_ = redis.SetDefaultActivityID(ctx, current.GoodsID, current.ID)
	}
	return redis.ClearSeckillData(ctx, id)
}

func (s *ActivityService) UpdateActivityStatus(ctx context.Context, id uint64, status uint8) error {
	activity, err := s.activityRepo.GetByID(id)
	if err != nil {
		return err
	}
	if err := validateActivityStatus(status); err != nil {
		return err
	}
	if err := s.ensureNoOverlap(activity.GoodsID, id, activity.StartTime, activity.EndTime, status); err != nil {
		return err
	}
	if err := s.activityRepo.UpdateStatus(id, status); err != nil {
		return err
	}
	return redis.ClearSeckillData(ctx, id)
}

func (s *ActivityService) WarmUpActivity(ctx context.Context, id uint64) error {
	if s.seckillSvc == nil {
		return errors.New("seckill service is required")
	}
	return s.seckillSvc.WarmUpActivity(ctx, id)
}

func (s *ActivityService) GetActivityResponse(id uint64) (*dto.ActivityResponse, error) {
	activity, err := s.activityRepo.GetWithGoods(id)
	if err != nil {
		return nil, err
	}
	resp := mapActivityResponse(*activity)
	return &resp, nil
}

func (s *ActivityService) defaultActivityWindow(now time.Time) (time.Time, time.Time, error) {
	startOffset, err := time.ParseDuration(strings.TrimSpace(s.cfg.Seckill.DefaultActivity.StartOffset))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: seckill.default_activity.start_offset", ErrActivityConfig)
	}
	endOffset, err := time.ParseDuration(strings.TrimSpace(s.cfg.Seckill.DefaultActivity.EndOffset))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: seckill.default_activity.end_offset", ErrActivityConfig)
	}

	startTime := now.Add(startOffset)
	endTime := now.Add(endOffset)
	if !endTime.After(startTime) {
		return time.Time{}, time.Time{}, ErrActivityTimeInvalid
	}
	return startTime, endTime, nil
}

func (s *ActivityService) defaultActivityTitle(goodsName string) (string, error) {
	template := strings.TrimSpace(s.cfg.Seckill.DefaultActivity.TitleTemplate)
	if template == "" {
		return "", fmt.Errorf("%w: seckill.default_activity.title_template", ErrActivityConfig)
	}
	return strings.ReplaceAll(template, "{goods_name}", strings.TrimSpace(goodsName)), nil
}

func (s *ActivityService) ensureNoOverlap(goodsID, excludeID uint64, startTime, endTime time.Time, status uint8) error {
	if status != model.SeckillActivityStatusPending && status != model.SeckillActivityStatusRunning {
		return nil
	}
	overlap, err := s.activityRepo.HasOverlappingEnabled(goodsID, excludeID, startTime, endTime)
	if err != nil {
		return err
	}
	if overlap {
		return ErrActivityOverlap
	}
	return nil
}

func parseActivityWindow(startRaw, endRaw string) (time.Time, time.Time, error) {
	startTime, err := parseActivityTime(startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	endTime, err := parseActivityTime(endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !endTime.After(startTime) {
		return time.Time{}, time.Time{}, ErrActivityTimeInvalid
	}
	return startTime, endTime, nil
}

func parseActivityTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, ErrActivityTimeInvalid
}

func validateActivityStatus(status uint8) error {
	switch status {
	case model.SeckillActivityStatusPending,
		model.SeckillActivityStatusRunning,
		model.SeckillActivityStatusEnded,
		model.SeckillActivityStatusDisabled:
		return nil
	default:
		return ErrActivityStatus
	}
}

func mapActivityResponses(items []repository.SeckillActivityWithGoods) []dto.ActivityResponse {
	resp := make([]dto.ActivityResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, mapActivityResponse(item))
	}
	return resp
}

func mapActivityResponse(item repository.SeckillActivityWithGoods) dto.ActivityResponse {
	return dto.ActivityResponse{
		ID:               item.ID,
		GoodsID:          item.GoodsID,
		Title:            item.Title,
		StartTime:        item.StartTime.Format(time.RFC3339),
		EndTime:          item.EndTime.Format(time.RFC3339),
		Status:           item.Status,
		MaxBuyLimit:      item.MaxBuyLimit,
		WarmupStatus:     item.WarmupStatus,
		IsDefault:        item.IsDefault,
		GoodsName:        item.GoodsName,
		GoodsDescription: item.GoodsDescription,
		GoodsPrice:       item.GoodsPrice,
		GoodsStatus:      item.GoodsStatus,
		GoodsStock:       item.GoodsStock,
	}
}
