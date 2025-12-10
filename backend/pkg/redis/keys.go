package redis

import "fmt"

const (
	keyPrefix       = "seckill:"
	stockKeyPrefix  = keyPrefix + "stock:"
	boughtKeyPrefix = keyPrefix + "bought:"
)

// StockKey 库存 key: seckill:stock:{goodsID}
func StockKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", stockKeyPrefix, goodsID)
}

// BoughtKey 已购用户集合 key: seckill:bought:{goodsID}
func BoughtKey(goodsID uint64) string {
	return fmt.Sprintf("%s%d", boughtKeyPrefix, goodsID)
}
