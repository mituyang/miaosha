package redis

import "fmt"

const (
	keyPrefix          = "seckill:"
	stockKeyPrefix     = keyPrefix + "stock:"
	boughtKeyPrefix    = keyPrefix + "bought:"
	deductedKeyPrefix  = keyPrefix + "deducted:"  // 已扣库存用户集合（废弃，保留兼容）
	processedKeyPrefix = keyPrefix + "processed:" // 已处理用户集合（Consumer 幂等）
	segmentKeyPrefix   = keyPrefix + "segment:"   // 分段库存
	SegmentCount       = 1                        // 库存分段数量
)

// StockKey 库存 key: seckill:stock:{goodsID}
func StockKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", stockKeyPrefix, goodsID)
}

// BoughtKey 已购用户集合 key: seckill:bought:{goodsID}
func BoughtKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", boughtKeyPrefix, goodsID)
}

// SegmentStockKey 分段库存 key: seckill:segment:{goodsID}:{segmentID}
func SegmentStockKey(goodsID uint64, segmentID int) string {
	return fmt.Sprintf("%s%d:%d", segmentKeyPrefix, goodsID, segmentID)
}

// DeductedKey 已扣库存用户集合 key: seckill:deducted:{goodsID}（废弃）
func DeductedKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", deductedKeyPrefix, goodsID)
}

// ProcessedKey 已处理用户集合 key: seckill:processed:{goodsID}
func ProcessedKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", processedKeyPrefix, goodsID)
}
