package dto

import "seckill/internal/repository"

type AdminGoodsUpsertRequest struct {
	ProductName string  `json:"product_name" binding:"required,min=1,max=255"`
	Description string  `json:"description" binding:"max=500"`
	Stock       int     `json:"stock" binding:"gte=0"`
	Price       float64 `json:"price" binding:"gte=0"`
	Status      uint8   `json:"status" binding:"oneof=0 1"`
}

type AdminUserStatusRequest struct {
	Status uint8 `json:"status" binding:"oneof=0 1"`
}

type AdminStatsResponse struct {
	OrderStats   *repository.OrderStats         `json:"order_stats"`
	UserStats    *repository.UserStats          `json:"user_stats"`
	GoodsStats   *repository.GoodsStats         `json:"goods_stats"`
	SalesRanking []repository.GoodsSalesRanking `json:"sales_ranking"`
}

type AdminObservabilityResponse struct {
	Redis      *AdminRedisObservability    `json:"redis,omitempty"`
	Kafka      *AdminKafkaObservability    `json:"kafka,omitempty"`
	ToolLinks  AdminObservabilityToolLinks `json:"tool_links"`
	RedisError string                      `json:"redis_error,omitempty"`
	KafkaError string                      `json:"kafka_error,omitempty"`
}

type AdminObservabilityToolLinks struct {
	AKHQ         string `json:"akhq"`
	RedisInsight string `json:"redis_insight"`
}

type AdminRedisObservability struct {
	DBSize              int64                    `json:"db_size"`
	AdminStatsReady     bool                     `json:"admin_stats_ready"`
	TimeoutQueueSize    int64                    `json:"timeout_queue_size"`
	PendingTimeoutCount int64                    `json:"pending_timeout_count"`
	Keyspace            AdminRedisKeyspace       `json:"keyspace"`
	WarmupLocks         []AdminRedisWarmupLock   `json:"warmup_locks"`
	Goods               []AdminRedisGoodsRuntime `json:"goods"`
}

type AdminRedisKeyspace struct {
	TotalKeys       int64 `json:"total_keys"`
	SegmentKeys     int64 `json:"segment_keys"`
	BoughtKeys      int64 `json:"bought_keys"`
	ProcessedKeys   int64 `json:"processed_keys"`
	GoodsStatusKeys int64 `json:"goods_status_keys"`
	UserStatusKeys  int64 `json:"user_status_keys"`
	AdminStatsKeys  int64 `json:"admin_stats_keys"`
}

type AdminRedisWarmupLock struct {
	Key    string `json:"key"`
	Target string `json:"target"`
	TTLSec int64  `json:"ttl_sec"`
}

type AdminRedisGoodsRuntime struct {
	GoodsID           uint64                   `json:"goods_id"`
	GoodsName         string                   `json:"goods_name"`
	OnSale            bool                     `json:"on_sale"`
	TotalStock        int64                    `json:"total_stock"`
	BoughtUsers       int64                    `json:"bought_users"`
	BoughtQuantity    int64                    `json:"bought_quantity"`
	ProcessedUsers    int64                    `json:"processed_users"`
	ProcessedQuantity int64                    `json:"processed_quantity"`
	PendingQuantity   int64                    `json:"pending_quantity"`
	SegmentStocks     []AdminRedisSegmentStock `json:"segment_stocks"`
}

type AdminRedisSegmentStock struct {
	SegmentID int   `json:"segment_id"`
	Stock     int64 `json:"stock"`
}

type AdminKafkaObservability struct {
	Brokers              []string                     `json:"brokers"`
	Topic                string                       `json:"topic"`
	Group                string                       `json:"group"`
	GroupState           string                       `json:"group_state"`
	PartitionCount       int                          `json:"partition_count"`
	ActiveMemberCount    int                          `json:"active_member_count"`
	TotalLatestOffset    int64                        `json:"total_latest_offset"`
	TotalCommittedOffset int64                        `json:"total_committed_offset"`
	TotalLag             int64                        `json:"total_lag"`
	DLQTopic             string                       `json:"dlq_topic"`
	DLQDepth             int64                        `json:"dlq_depth"`
	Members              []AdminKafkaMemberRuntime    `json:"members"`
	Partitions           []AdminKafkaPartitionRuntime `json:"partitions"`
}

type AdminKafkaMemberRuntime struct {
	MemberID   string `json:"member_id"`
	ClientID   string `json:"client_id"`
	ClientHost string `json:"client_host"`
	Partitions []int  `json:"partitions"`
}

type AdminKafkaPartitionRuntime struct {
	Partition       int    `json:"partition"`
	Leader          string `json:"leader"`
	EarliestOffset  int64  `json:"earliest_offset"`
	LatestOffset    int64  `json:"latest_offset"`
	CommittedOffset int64  `json:"committed_offset"`
	Lag             int64  `json:"lag"`
	MemberID        string `json:"member_id,omitempty"`
	ClientID        string `json:"client_id,omitempty"`
	ClientHost      string `json:"client_host,omitempty"`
}

type PageResponse struct {
	Items    interface{} `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}
