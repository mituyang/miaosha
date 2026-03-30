package service

import (
	"context"
	"errors"
	"strings"

	"seckill/internal/dto"
	"seckill/internal/model"
	"seckill/internal/repository"
	"seckill/pkg/database"
	"seckill/pkg/redis"
)

var (
	ErrGoodsHasOrders = errors.New("goods has orders")
	ErrInvalidStatus  = errors.New("invalid status")
)

type AdminService struct {
	goodsRepo  *repository.GoodsRepository
	orderRepo  *repository.OrderRepository
	userRepo   *repository.UserRepository
	seckillSvc *SeckillService
}

func NewAdminService(seckillSvc *SeckillService) *AdminService {
	return &AdminService{
		goodsRepo:  repository.NewGoodsRepository(database.DB),
		orderRepo:  repository.NewOrderRepository(database.DB),
		userRepo:   repository.NewUserRepository(),
		seckillSvc: seckillSvc,
	}
}

// ListGoods 查询商品列表
func (s *AdminService) ListGoods(keyword string, status *uint8, page, pageSize int) ([]model.Goods, int64, error) {
	return s.goodsRepo.ListPage(repository.GoodsFilter{
		Keyword: strings.TrimSpace(keyword),
		Status:  status,
	}, page, pageSize)
}

// CreateGoods 创建商品
func (s *AdminService) CreateGoods(ctx context.Context, req dto.AdminGoodsUpsertRequest) (*model.Goods, error) {
	goods := &model.Goods{
		ProductName: strings.TrimSpace(req.ProductName),
		Description: strings.TrimSpace(req.Description),
		Stock:       uint(req.Stock),
		Price:       req.Price,
		Status:      req.Status,
	}
	if goods.Status != model.GoodsStatusOffShelf && goods.Status != model.GoodsStatusOnSale {
		return nil, ErrInvalidStatus
	}

	if err := s.goodsRepo.Create(goods); err != nil {
		return nil, err
	}
	if err := redis.SetGoodsOnSale(ctx, goods.ID, goods.Status == model.GoodsStatusOnSale); err != nil {
		return nil, err
	}
	_ = redis.IncrementAdminGoodsCreated(ctx, goods.ID, req.Stock, goods.Status, goods.ProductName)
	return goods, nil
}

// UpdateGoods 更新商品
func (s *AdminService) UpdateGoods(ctx context.Context, goodsID uint64, req dto.AdminGoodsUpsertRequest) error {
	goods, err := s.goodsRepo.GetByID(goodsID)
	if err != nil {
		return err
	}
	currentStock := goods.Stock
	currentStatus := goods.Status

	if req.Status != model.GoodsStatusOffShelf && req.Status != model.GoodsStatusOnSale {
		return ErrInvalidStatus
	}

	goods.ProductName = strings.TrimSpace(req.ProductName)
	goods.Description = strings.TrimSpace(req.Description)
	goods.Stock = uint(req.Stock)
	goods.Price = req.Price
	goods.Status = req.Status

	if err := s.goodsRepo.Update(goods); err != nil {
		return err
	}

	if err := redis.SetGoodsOnSale(ctx, goodsID, goods.Status == model.GoodsStatusOnSale); err != nil {
		return err
	}
	if goods.Status == model.GoodsStatusOffShelf {
		if err := redis.ClearSeckillData(ctx, goodsID); err != nil {
			return err
		}
	}

	_ = redis.AdjustAdminGoodsUpdated(
		ctx,
		goodsID,
		int(currentStock),
		req.Stock,
		currentStatus,
		goods.Status,
		goods.ProductName,
	)
	return nil
}

// DeleteGoods 删除商品
func (s *AdminService) DeleteGoods(ctx context.Context, goodsID uint64) error {
	goods, err := s.goodsRepo.GetByID(goodsID)
	if err != nil {
		return err
	}

	hasOrders, err := s.goodsRepo.HasOrders(goodsID)
	if err != nil {
		return err
	}
	if hasOrders {
		return ErrGoodsHasOrders
	}

	if err := s.goodsRepo.Delete(goodsID); err != nil {
		return err
	}
	if err := redis.ClearSeckillData(ctx, goodsID); err != nil {
		return err
	}
	_ = redis.IncrementAdminGoodsDeleted(ctx, goodsID, int(goods.Stock), goods.Status)
	return redis.ClearGoodsOnSale(ctx, goodsID)
}

// ListOrders 查询订单列表
func (s *AdminService) ListOrders(keyword string, status *uint8, page, pageSize int) ([]repository.AdminOrderItem, int64, error) {
	return s.orderRepo.ListPage(repository.OrderFilter{
		Keyword: strings.TrimSpace(keyword),
		Status:  status,
	}, page, pageSize)
}

// GetOrderDetail 查询订单详情
func (s *AdminService) GetOrderDetail(orderID uint64) (*repository.AdminOrderItem, error) {
	return s.orderRepo.GetDetail(orderID)
}

// ListUsers 查询用户列表
func (s *AdminService) ListUsers(keyword string, status *uint8, page, pageSize int) ([]repository.AdminUserItem, int64, error) {
	return s.userRepo.ListPage(repository.UserFilter{
		Keyword: strings.TrimSpace(keyword),
		Status:  status,
	}, page, pageSize)
}

// UpdateUserStatus 更新用户状态
func (s *AdminService) UpdateUserStatus(ctx context.Context, userID uint64, status uint8) error {
	if status != model.UserStatusDisabled && status != model.UserStatusEnabled {
		return ErrInvalidStatus
	}

	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	currentStatus := user.Status

	user.Status = status
	if err := s.userRepo.UpdateStatus(userID, status); err != nil {
		return err
	}

	_ = redis.AdjustAdminUserStatus(ctx, currentStatus, status)
	return redis.SetUserEnabled(ctx, user.ID, status == model.UserStatusEnabled)
}

// WarmUpGoods 预热单商品
func (s *AdminService) WarmUpGoods(ctx context.Context, goodsID uint64) error {
	return s.seckillSvc.WarmUp(ctx, goodsID)
}

// WarmUpAll 全量预热
func (s *AdminService) WarmUpAll(ctx context.Context) (int, error) {
	return s.seckillSvc.WarmUpAll(ctx)
}

// Ping 校验管理员凭证
func (s *AdminService) Ping() bool {
	return true
}

// WarmStatsCache 启动时预热后台统计快照
func (s *AdminService) WarmStatsCache(ctx context.Context) error {
	_, err := s.reloadStatsSnapshot(ctx)
	return err
}

// GetStats 查询后台统计
func (s *AdminService) GetStats(ctx context.Context) (*dto.AdminStatsResponse, error) {
	orderStats, userStats, goodsStats, salesRanking, ok, err := redis.GetAdminStatsSnapshot(ctx, 10)
	if err == nil && ok {
		return &dto.AdminStatsResponse{
			OrderStats:   orderStats,
			UserStats:    userStats,
			GoodsStats:   goodsStats,
			SalesRanking: salesRanking,
		}, nil
	}

	return s.reloadStatsSnapshot(ctx)
}

// RebuildStats 强制从 MySQL 重建后台统计快照
func (s *AdminService) RebuildStats(ctx context.Context) (*dto.AdminStatsResponse, error) {
	return s.reloadStatsSnapshot(ctx)
}

func (s *AdminService) reloadStatsSnapshot(ctx context.Context) (*dto.AdminStatsResponse, error) {
	orderStats, err := s.orderRepo.GetStats()
	if err != nil {
		return nil, err
	}

	userStats, err := s.userRepo.GetStats()
	if err != nil {
		return nil, err
	}

	goodsStats, err := s.goodsRepo.GetStats()
	if err != nil {
		return nil, err
	}

	allGoods, err := s.goodsRepo.GetAll()
	if err != nil {
		return nil, err
	}

	allSalesRanking, err := s.orderRepo.GetSalesRanking(0)
	if err != nil {
		return nil, err
	}

	goodsNames := make(map[uint64]string, len(allGoods))
	for _, item := range allGoods {
		goodsNames[item.ID] = item.ProductName
	}

	_ = redis.SetAdminStatsSnapshot(ctx, orderStats, userStats, goodsStats, goodsNames, allSalesRanking)

	respRanking := allSalesRanking
	if len(respRanking) > 10 {
		respRanking = respRanking[:10]
	}

	return &dto.AdminStatsResponse{
		OrderStats:   orderStats,
		UserStats:    userStats,
		GoodsStats:   goodsStats,
		SalesRanking: respRanking,
	}, nil
}
