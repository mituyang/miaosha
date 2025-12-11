package redis

import "fmt"

const (
	keyPrefix         = "seckill:"
	stockKeyPrefix    = keyPrefix + "stock:"
	boughtKeyPrefix   = keyPrefix + "bought:"
	deductedKeyPrefix = keyPrefix + "deducted:" // 已扣库存用户集合
	segmentKeyPrefix  = keyPrefix + "segment:"  // 分段库存
	SegmentCount      = 10                      // 库存分段数量
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

// DeductedKey 已扣库存用户集合 key: seckill:deducted:{goodsID}
func DeductedKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", deductedKeyPrefix, goodsID)
}
