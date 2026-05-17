package redis

import (
	"context"
	"strconv"

	redisLib "github.com/redis/go-redis/v9"
)

type ActivityMeta struct {
	ActivityID   uint64
	GoodsID      uint64
	Title        string
	Status       uint8
	StartTimeMs  int64
	EndTimeMs    int64
	MaxBuyLimit  int
	WarmupStatus uint8
	GoodsOnSale  bool
	IsDefault    uint8
}

// SetGoodsOnSale 设置商品上下架状态
func SetGoodsOnSale(ctx context.Context, goodsID uint64, onSale bool) error {
	value := "0"
	if onSale {
		value = "1"
	}
	return Client.Set(ctx, GoodsStatusKey(goodsID), value, 0).Err()
}

// IsGoodsOnSale 查询商品是否上架，缓存缺失时按未上架处理
func IsGoodsOnSale(ctx context.Context, goodsID uint64) (bool, error) {
	value, err := Client.Get(ctx, GoodsStatusKey(goodsID)).Result()
	if err != nil {
		if err == redisLib.Nil {
			return false, nil
		}
		return false, err
	}
	return value == "1", nil
}

// ClearGoodsOnSale 清理商品上下架状态缓存
func ClearGoodsOnSale(ctx context.Context, goodsID uint64) error {
	return Client.Del(ctx, GoodsStatusKey(goodsID)).Err()
}

// SetActivityMeta 写入活动热路径元数据
func SetActivityMeta(ctx context.Context, meta ActivityMeta) error {
	goodsOnSale := "0"
	if meta.GoodsOnSale {
		goodsOnSale = "1"
	}

	pipe := Client.Pipeline()
	pipe.HSet(ctx, ActivityMetaKey(meta.ActivityID), map[string]interface{}{
		"activity_id":   meta.ActivityID,
		"goods_id":      meta.GoodsID,
		"title":         meta.Title,
		"status":        meta.Status,
		"start_time":    meta.StartTimeMs,
		"end_time":      meta.EndTimeMs,
		"max_buy_limit": meta.MaxBuyLimit,
		"warmup_status": meta.WarmupStatus,
		"goods_on_sale": goodsOnSale,
		"is_default":    meta.IsDefault,
	})
	if meta.IsDefault == 1 {
		pipe.Set(ctx, DefaultActivityKey(meta.GoodsID), strconv.FormatUint(meta.ActivityID, 10), 0)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// GetActivityMeta 读取活动热路径元数据，返回 exists=false 表示未预热
func GetActivityMeta(ctx context.Context, activityID uint64) (*ActivityMeta, bool, error) {
	values, err := Client.HGetAll(ctx, ActivityMetaKey(activityID)).Result()
	if err != nil {
		return nil, false, err
	}
	if len(values) == 0 {
		return nil, false, nil
	}

	goodsID, _ := strconv.ParseUint(values["goods_id"], 10, 64)
	status, _ := strconv.ParseUint(values["status"], 10, 8)
	startTimeMs, _ := strconv.ParseInt(values["start_time"], 10, 64)
	endTimeMs, _ := strconv.ParseInt(values["end_time"], 10, 64)
	maxBuyLimit, _ := strconv.Atoi(values["max_buy_limit"])
	warmupStatus, _ := strconv.ParseUint(values["warmup_status"], 10, 8)
	isDefault, _ := strconv.ParseUint(values["is_default"], 10, 8)

	return &ActivityMeta{
		ActivityID:   activityID,
		GoodsID:      goodsID,
		Title:        values["title"],
		Status:       uint8(status),
		StartTimeMs:  startTimeMs,
		EndTimeMs:    endTimeMs,
		MaxBuyLimit:  maxBuyLimit,
		WarmupStatus: uint8(warmupStatus),
		GoodsOnSale:  values["goods_on_sale"] == "1",
		IsDefault:    uint8(isDefault),
	}, true, nil
}

// SetDefaultActivityID 写入商品默认活动映射
func SetDefaultActivityID(ctx context.Context, goodsID, activityID uint64) error {
	return Client.Set(ctx, DefaultActivityKey(goodsID), strconv.FormatUint(activityID, 10), 0).Err()
}

// GetDefaultActivityID 读取商品默认活动映射
func GetDefaultActivityID(ctx context.Context, goodsID uint64) (uint64, bool, error) {
	value, err := Client.Get(ctx, DefaultActivityKey(goodsID)).Result()
	if err != nil {
		if err == redisLib.Nil {
			return 0, false, nil
		}
		return 0, false, err
	}
	activityID, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return activityID, true, nil
}

// ClearActivityMeta 清理活动元数据
func ClearActivityMeta(ctx context.Context, activityID uint64) error {
	return Client.Del(ctx, ActivityMetaKey(activityID)).Err()
}

// ClearDefaultActivityID 清理商品默认活动映射
func ClearDefaultActivityID(ctx context.Context, goodsID uint64) error {
	return Client.Del(ctx, DefaultActivityKey(goodsID)).Err()
}

// SetUserEnabled 设置用户状态
func SetUserEnabled(ctx context.Context, userID uint64, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	return Client.Set(ctx, UserStatusKey(userID), value, 0).Err()
}

// IsUserEnabled 查询用户是否启用，缓存缺失时默认启用以兼容旧数据
func IsUserEnabled(ctx context.Context, userID uint64) (bool, error) {
	value, err := Client.Get(ctx, UserStatusKey(userID)).Result()
	if err != nil {
		if err == redisLib.Nil {
			return true, nil
		}
		return false, err
	}
	return value == "1", nil
}
