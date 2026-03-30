package redis

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	redisLib "github.com/redis/go-redis/v9"

	"seckill/internal/model"
	"seckill/internal/repository"
)

const (
	adminStatsReadyKey        = "admin:stats:ready"
	adminStatsDirtyKey        = "admin:stats:dirty"
	adminOrderStatsKey        = "admin:stats:orders"
	adminUserStatsKey         = "admin:stats:users"
	adminGoodsStatsKey        = "admin:stats:goods"
	adminGoodsNamesKey        = "admin:stats:goods:names"
	adminRankingSoldKey       = "admin:stats:ranking:sold"
	adminRankingOrderKey      = "admin:stats:ranking:orders"
	adminRankingSalesKey      = "admin:stats:ranking:sales"
	adminRankingZSetKey       = "admin:stats:ranking:zset"
	adminFieldStatsDay        = "stats_day"
	adminFieldTotalOrders     = "total_orders"
	adminFieldPaidOrders      = "paid_orders"
	adminFieldUnpaidOrders    = "unpaid_orders"
	adminFieldCancelledOrders = "cancelled_orders"
	adminFieldTotalSales      = "total_sales"
	adminFieldTodayPaidOrders = "today_paid_orders"
	adminFieldTodaySales      = "today_sales_amount"
	adminFieldTotalUsers      = "total_users"
	adminFieldEnabledUsers    = "enabled_users"
	adminFieldDisabledUsers   = "disabled_users"
	adminFieldTotalGoods      = "total_goods"
	adminFieldOnSaleGoods     = "on_sale_goods"
	adminFieldTotalStock      = "total_stock"
)

// AdminStatsReady 判断后台统计快照是否可读
func AdminStatsReady(ctx context.Context) (bool, error) {
	ready, err := Client.Exists(ctx, adminStatsReadyKey).Result()
	if err != nil {
		return false, err
	}
	if ready == 0 {
		return false, nil
	}

	dirty, err := Client.Exists(ctx, adminStatsDirtyKey).Result()
	if err != nil {
		return false, err
	}

	return dirty == 0, nil
}

// SetAdminStatsSnapshot 全量写入后台统计快照
func SetAdminStatsSnapshot(
	ctx context.Context,
	orderStats *repository.OrderStats,
	userStats *repository.UserStats,
	goodsStats *repository.GoodsStats,
	goodsNames map[uint64]string,
	salesRanking []repository.GoodsSalesRanking,
) error {
	pipe := Client.Pipeline()
	pipe.Del(
		ctx,
		adminStatsReadyKey,
		adminStatsDirtyKey,
		adminOrderStatsKey,
		adminUserStatsKey,
		adminGoodsStatsKey,
		adminGoodsNamesKey,
		adminRankingSoldKey,
		adminRankingOrderKey,
		adminRankingSalesKey,
		adminRankingZSetKey,
	)

	if orderStats != nil {
		pipe.HSet(ctx, adminOrderStatsKey, map[string]interface{}{
			adminFieldStatsDay:        adminStatsDay(time.Now()),
			adminFieldTotalOrders:     orderStats.TotalOrders,
			adminFieldPaidOrders:      orderStats.PaidOrders,
			adminFieldUnpaidOrders:    orderStats.UnpaidOrders,
			adminFieldCancelledOrders: orderStats.CancelledOrders,
			adminFieldTotalSales:      formatFloat(orderStats.TotalSales),
			adminFieldTodayPaidOrders: orderStats.TodayPaidOrders,
			adminFieldTodaySales:      formatFloat(orderStats.TodaySalesAmount),
		})
	}

	if userStats != nil {
		pipe.HSet(ctx, adminUserStatsKey, map[string]interface{}{
			adminFieldTotalUsers:    userStats.TotalUsers,
			adminFieldEnabledUsers:  userStats.EnabledUsers,
			adminFieldDisabledUsers: userStats.DisabledUsers,
		})
	}

	if goodsStats != nil {
		pipe.HSet(ctx, adminGoodsStatsKey, map[string]interface{}{
			adminFieldTotalGoods:  goodsStats.TotalGoods,
			adminFieldOnSaleGoods: goodsStats.OnSaleGoods,
			adminFieldTotalStock:  goodsStats.TotalStock,
		})
	}

	if len(goodsNames) > 0 {
		nameMap := make(map[string]interface{}, len(goodsNames))
		for goodsID, goodsName := range goodsNames {
			nameMap[strconv.FormatUint(goodsID, 10)] = goodsName
		}
		pipe.HSet(ctx, adminGoodsNamesKey, nameMap)
	}

	if len(salesRanking) > 0 {
		soldMap := make(map[string]interface{}, len(salesRanking))
		orderMap := make(map[string]interface{}, len(salesRanking))
		salesMap := make(map[string]interface{}, len(salesRanking))
		zMembers := make([]redisLib.Z, 0, len(salesRanking))

		for _, item := range salesRanking {
			member := strconv.FormatUint(item.GoodsID, 10)
			soldMap[member] = item.SoldQuantity
			orderMap[member] = item.OrderCount
			salesMap[member] = formatFloat(item.SalesAmount)
			zMembers = append(zMembers, redisLib.Z{
				Score:  float64(item.SoldQuantity),
				Member: member,
			})
		}

		pipe.HSet(ctx, adminRankingSoldKey, soldMap)
		pipe.HSet(ctx, adminRankingOrderKey, orderMap)
		pipe.HSet(ctx, adminRankingSalesKey, salesMap)
		pipe.ZAdd(ctx, adminRankingZSetKey, zMembers...)
	}

	pipe.Set(ctx, adminStatsReadyKey, "1", 0)
	_, err := pipe.Exec(ctx)
	return err
}

// GetAdminStatsSnapshot 从 Redis 读取后台统计快照
func GetAdminStatsSnapshot(ctx context.Context, limit int) (*repository.OrderStats, *repository.UserStats, *repository.GoodsStats, []repository.GoodsSalesRanking, bool, error) {
	ready, err := AdminStatsReady(ctx)
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	if !ready {
		return nil, nil, nil, nil, false, nil
	}

	if err := resetAdminTodayStatsIfNeeded(ctx, time.Now()); err != nil {
		return nil, nil, nil, nil, false, err
	}

	if limit <= 0 {
		limit = 10
	}

	pipe := Client.Pipeline()
	orderCmd := pipe.HMGet(
		ctx,
		adminOrderStatsKey,
		adminFieldTotalOrders,
		adminFieldPaidOrders,
		adminFieldUnpaidOrders,
		adminFieldCancelledOrders,
		adminFieldTotalSales,
		adminFieldTodayPaidOrders,
		adminFieldTodaySales,
	)
	userCmd := pipe.HMGet(
		ctx,
		adminUserStatsKey,
		adminFieldTotalUsers,
		adminFieldEnabledUsers,
		adminFieldDisabledUsers,
	)
	goodsCmd := pipe.HMGet(
		ctx,
		adminGoodsStatsKey,
		adminFieldTotalGoods,
		adminFieldOnSaleGoods,
		adminFieldTotalStock,
	)
	rankingCmd := pipe.ZRevRange(ctx, adminRankingZSetKey, 0, int64(limit-1))
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, nil, nil, nil, false, err
	}

	orderStats := &repository.OrderStats{
		TotalOrders:      parseInt64(orderCmd.Val()[0]),
		PaidOrders:       parseInt64(orderCmd.Val()[1]),
		UnpaidOrders:     parseInt64(orderCmd.Val()[2]),
		CancelledOrders:  parseInt64(orderCmd.Val()[3]),
		TotalSales:       parseFloat64(orderCmd.Val()[4]),
		TodayPaidOrders:  parseInt64(orderCmd.Val()[5]),
		TodaySalesAmount: parseFloat64(orderCmd.Val()[6]),
	}

	userStats := &repository.UserStats{
		TotalUsers:    parseInt64(userCmd.Val()[0]),
		EnabledUsers:  parseInt64(userCmd.Val()[1]),
		DisabledUsers: parseInt64(userCmd.Val()[2]),
	}

	goodsStats := &repository.GoodsStats{
		TotalGoods:  parseInt64(goodsCmd.Val()[0]),
		OnSaleGoods: parseInt64(goodsCmd.Val()[1]),
		TotalStock:  parseInt64(goodsCmd.Val()[2]),
	}

	salesRanking, err := getAdminSalesRanking(ctx, rankingCmd.Val())
	if err != nil {
		return nil, nil, nil, nil, false, err
	}

	return orderStats, userStats, goodsStats, salesRanking, true, nil
}

// ClearAdminStatsCache 清空后台统计快照
func ClearAdminStatsCache(ctx context.Context) error {
	return Client.Del(
		ctx,
		adminStatsReadyKey,
		adminStatsDirtyKey,
		adminOrderStatsKey,
		adminUserStatsKey,
		adminGoodsStatsKey,
		adminGoodsNamesKey,
		adminRankingSoldKey,
		adminRankingOrderKey,
		adminRankingSalesKey,
		adminRankingZSetKey,
	).Err()
}

// IncrementAdminUserCreated 新增用户统计
func IncrementAdminUserCreated(ctx context.Context, status uint8) error {
	if !adminStatsWritable(ctx) {
		return nil
	}

	pipe := Client.Pipeline()
	pipe.HIncrBy(ctx, adminUserStatsKey, adminFieldTotalUsers, 1)
	if status == model.UserStatusDisabled {
		pipe.HIncrBy(ctx, adminUserStatsKey, adminFieldDisabledUsers, 1)
	} else {
		pipe.HIncrBy(ctx, adminUserStatsKey, adminFieldEnabledUsers, 1)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// AdjustAdminUserStatus 调整用户状态统计
func AdjustAdminUserStatus(ctx context.Context, beforeStatus, afterStatus uint8) error {
	if beforeStatus == afterStatus || !adminStatsWritable(ctx) {
		return nil
	}

	pipe := Client.Pipeline()
	switch beforeStatus {
	case model.UserStatusDisabled:
		pipe.HIncrBy(ctx, adminUserStatsKey, adminFieldDisabledUsers, -1)
	default:
		pipe.HIncrBy(ctx, adminUserStatsKey, adminFieldEnabledUsers, -1)
	}

	switch afterStatus {
	case model.UserStatusDisabled:
		pipe.HIncrBy(ctx, adminUserStatsKey, adminFieldDisabledUsers, 1)
	default:
		pipe.HIncrBy(ctx, adminUserStatsKey, adminFieldEnabledUsers, 1)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// IncrementAdminGoodsCreated 新增商品统计
func IncrementAdminGoodsCreated(ctx context.Context, goodsID uint64, stock int, status uint8, goodsName string) error {
	if !adminStatsWritable(ctx) {
		return nil
	}

	pipe := Client.Pipeline()
	pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldTotalGoods, 1)
	pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldTotalStock, int64(stock))
	if status == model.GoodsStatusOnSale {
		pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldOnSaleGoods, 1)
	}
	pipe.HSet(ctx, adminGoodsNamesKey, strconv.FormatUint(goodsID, 10), goodsName)
	_, err := pipe.Exec(ctx)
	return err
}

// AdjustAdminGoodsUpdated 调整商品统计
func AdjustAdminGoodsUpdated(ctx context.Context, goodsID uint64, beforeStock, afterStock int, beforeStatus, afterStatus uint8, goodsName string) error {
	if !adminStatsWritable(ctx) {
		return nil
	}

	pipe := Client.Pipeline()
	stockDelta := afterStock - beforeStock
	if stockDelta != 0 {
		pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldTotalStock, int64(stockDelta))
	}

	if beforeStatus != afterStatus {
		if beforeStatus == model.GoodsStatusOnSale {
			pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldOnSaleGoods, -1)
		}
		if afterStatus == model.GoodsStatusOnSale {
			pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldOnSaleGoods, 1)
		}
	}

	pipe.HSet(ctx, adminGoodsNamesKey, strconv.FormatUint(goodsID, 10), goodsName)
	_, err := pipe.Exec(ctx)
	return err
}

// IncrementAdminGoodsDeleted 删除商品统计
func IncrementAdminGoodsDeleted(ctx context.Context, goodsID uint64, stock int, status uint8) error {
	if !adminStatsWritable(ctx) {
		return nil
	}

	pipe := Client.Pipeline()
	pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldTotalGoods, -1)
	pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldTotalStock, int64(-stock))
	if status == model.GoodsStatusOnSale {
		pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldOnSaleGoods, -1)
	}
	pipe.HDel(ctx, adminGoodsNamesKey, strconv.FormatUint(goodsID, 10))
	_, err := pipe.Exec(ctx)
	return err
}

// IncrementAdminOrdersCreated 订单创建后增量更新统计
func IncrementAdminOrdersCreated(ctx context.Context, orders []*model.Order) error {
	if len(orders) == 0 || !adminStatsWritable(ctx) {
		return nil
	}

	var totalStockDelta int64
	for _, order := range orders {
		quantity := order.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		totalStockDelta += int64(quantity)
	}

	pipe := Client.Pipeline()
	pipe.HIncrBy(ctx, adminOrderStatsKey, adminFieldTotalOrders, int64(len(orders)))
	pipe.HIncrBy(ctx, adminOrderStatsKey, adminFieldUnpaidOrders, int64(len(orders)))
	pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldTotalStock, -totalStockDelta)
	_, err := pipe.Exec(ctx)
	return err
}

// MarkAdminOrderPaid 支付后增量更新统计
func MarkAdminOrderPaid(ctx context.Context, goodsID uint64, quantity int, payAmount float64, payTime time.Time) error {
	if !adminStatsWritable(ctx) {
		return nil
	}

	if quantity <= 0 {
		quantity = 1
	}
	if err := resetAdminTodayStatsIfNeeded(ctx, payTime); err != nil {
		return err
	}

	member := strconv.FormatUint(goodsID, 10)
	pipe := Client.Pipeline()
	pipe.HIncrBy(ctx, adminOrderStatsKey, adminFieldPaidOrders, 1)
	pipe.HIncrBy(ctx, adminOrderStatsKey, adminFieldUnpaidOrders, -1)
	pipe.HIncrByFloat(ctx, adminOrderStatsKey, adminFieldTotalSales, payAmount)
	pipe.HIncrBy(ctx, adminOrderStatsKey, adminFieldTodayPaidOrders, 1)
	pipe.HIncrByFloat(ctx, adminOrderStatsKey, adminFieldTodaySales, payAmount)
	pipe.HIncrBy(ctx, adminRankingSoldKey, member, int64(quantity))
	pipe.HIncrBy(ctx, adminRankingOrderKey, member, 1)
	pipe.HIncrByFloat(ctx, adminRankingSalesKey, member, payAmount)
	pipe.ZIncrBy(ctx, adminRankingZSetKey, float64(quantity), member)
	_, err := pipe.Exec(ctx)
	return err
}

// MarkAdminOrderCancelled 取消订单后增量更新统计
func MarkAdminOrderCancelled(ctx context.Context, goodsID uint64, quantity int) error {
	if !adminStatsWritable(ctx) {
		return nil
	}

	if quantity <= 0 {
		quantity = 1
	}

	pipe := Client.Pipeline()
	pipe.HIncrBy(ctx, adminOrderStatsKey, adminFieldUnpaidOrders, -1)
	pipe.HIncrBy(ctx, adminOrderStatsKey, adminFieldCancelledOrders, 1)
	pipe.HIncrBy(ctx, adminGoodsStatsKey, adminFieldTotalStock, int64(quantity))
	_, err := pipe.Exec(ctx)
	return err
}

func adminStatsWritable(ctx context.Context) bool {
	ready, err := Client.Exists(ctx, adminStatsReadyKey).Result()
	if err != nil || ready == 0 {
		_ = Client.Set(ctx, adminStatsDirtyKey, "1", 0).Err()
		return false
	}
	return true
}

func resetAdminTodayStatsIfNeeded(ctx context.Context, now time.Time) error {
	day, err := Client.HGet(ctx, adminOrderStatsKey, adminFieldStatsDay).Result()
	if err != nil {
		if err == redisLib.Nil {
			return nil
		}
		return err
	}

	today := adminStatsDay(now)
	if day == today {
		return nil
	}

	pipe := Client.Pipeline()
	pipe.HSet(ctx, adminOrderStatsKey, map[string]interface{}{
		adminFieldStatsDay:        today,
		adminFieldTodayPaidOrders: 0,
		adminFieldTodaySales:      "0",
	})
	_, err = pipe.Exec(ctx)
	return err
}

func getAdminSalesRanking(ctx context.Context, goodsIDs []string) ([]repository.GoodsSalesRanking, error) {
	if len(goodsIDs) == 0 {
		return []repository.GoodsSalesRanking{}, nil
	}

	pipe := Client.Pipeline()
	soldCmd := pipe.HMGet(ctx, adminRankingSoldKey, goodsIDs...)
	orderCmd := pipe.HMGet(ctx, adminRankingOrderKey, goodsIDs...)
	salesCmd := pipe.HMGet(ctx, adminRankingSalesKey, goodsIDs...)
	nameCmd := pipe.HMGet(ctx, adminGoodsNamesKey, goodsIDs...)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	ranking := make([]repository.GoodsSalesRanking, 0, len(goodsIDs))
	for i, goodsIDStr := range goodsIDs {
		goodsID, _ := strconv.ParseUint(goodsIDStr, 10, 64)
		item := repository.GoodsSalesRanking{
			GoodsID:      goodsID,
			GoodsName:    parseString(nameCmd.Val()[i]),
			SoldQuantity: parseInt64(soldCmd.Val()[i]),
			OrderCount:   parseInt64(orderCmd.Val()[i]),
			SalesAmount:  parseFloat64(salesCmd.Val()[i]),
		}
		if item.GoodsName == "" {
			item.GoodsName = "已删除商品"
		}
		if item.SoldQuantity <= 0 || item.OrderCount <= 0 {
			continue
		}
		ranking = append(ranking, item)
	}

	sort.Slice(ranking, func(i, j int) bool {
		if ranking[i].SoldQuantity != ranking[j].SoldQuantity {
			return ranking[i].SoldQuantity > ranking[j].SoldQuantity
		}
		if ranking[i].SalesAmount != ranking[j].SalesAmount {
			return ranking[i].SalesAmount > ranking[j].SalesAmount
		}
		return ranking[i].GoodsID < ranking[j].GoodsID
	})

	return ranking, nil
}

func adminStatsDay(now time.Time) string {
	return now.In(time.Local).Format("2006-01-02")
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func parseInt64(value interface{}) int64 {
	text := parseString(value)
	if text == "" {
		return 0
	}
	parsed, _ := strconv.ParseInt(text, 10, 64)
	return parsed
}

func parseFloat64(value interface{}) float64 {
	text := parseString(value)
	if text == "" {
		return 0
	}
	parsed, _ := strconv.ParseFloat(text, 64)
	return parsed
}

func parseString(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}
