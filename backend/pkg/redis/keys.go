package redis

import "fmt"

const (
	keyPrefix             = "seckill:"
	boughtKeyPrefix       = keyPrefix + "activity:bought:"
	deductedKeyPrefix     = keyPrefix + "deducted:" // 已扣库存用户集合（废弃，保留兼容）
	processedKeyPrefix    = keyPrefix + "activity:processed:"
	segmentKeyPrefix      = keyPrefix + "activity:segment:"
	goodsStatusPrefix     = keyPrefix + "goods:status:"
	activityMetaPrefix    = keyPrefix + "activity:meta:"
	defaultActivityPrefix = keyPrefix + "goods:default_activity:"
	userStatusPrefix      = keyPrefix + "user:status:"
)

// SegmentCount 库存分段数量，从配置读取
var SegmentCount = 32

// SetSegmentCount 设置库存分段数量（从配置初始化）
func SetSegmentCount(count int) {
	if count > 0 {
		SegmentCount = count
	}
}

// BoughtKey 已购用户集合 key: seckill:activity:bought:{activityID}
func BoughtKey(activityID uint64) string {
	return fmt.Sprintf("%s%d", boughtKeyPrefix, activityID)
}

// SegmentStockKey 分段库存 key: seckill:activity:segment:{activityID}:{segmentID}
func SegmentStockKey(activityID uint64, segmentID int) string {
	return fmt.Sprintf("%s%d:%d", segmentKeyPrefix, activityID, segmentID)
}

// DeductedKey 已扣库存用户集合 key: seckill:deducted:{goodsID}（废弃）
func DeductedKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", deductedKeyPrefix, goodsID)
}

// ProcessedKey 已处理用户集合 key: seckill:activity:processed:{activityID}
func ProcessedKey(activityID uint64) string {
	return fmt.Sprintf("%s%d", processedKeyPrefix, activityID)
}

// GoodsStatusKey 商品上下架状态 key: seckill:goods:status:{goodsID}
func GoodsStatusKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", goodsStatusPrefix, goodsID)
}

// ActivityMetaKey 活动元数据 key: seckill:activity:meta:{activityID}
func ActivityMetaKey(activityID uint64) string {
	return fmt.Sprintf("%s%d", activityMetaPrefix, activityID)
}

// DefaultActivityKey 商品默认活动映射 key: seckill:goods:default_activity:{goodsID}
func DefaultActivityKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", defaultActivityPrefix, goodsID)
}

// UserStatusKey 用户状态 key: seckill:user:status:{userID}
func UserStatusKey(userID uint64) string {
	return fmt.Sprintf("%s%d", userStatusPrefix, userID)
}
