package service

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	redisLib "github.com/redis/go-redis/v9"
	kafkaLib "github.com/segmentio/kafka-go"

	"seckill/internal/dto"
	"seckill/internal/model"
	"seckill/pkg/redis"
)

const (
	akhqURL          = "http://localhost:8086"
	redisInsightURL  = "http://localhost:5540"
	warmupLockPrefix = "seckill:warmup:lock:"
)

// GetObservability 查询 Kafka / Redis 运行态
func (s *AdminService) GetObservability(ctx context.Context) (*dto.AdminObservabilityResponse, error) {
	resp := &dto.AdminObservabilityResponse{
		ToolLinks: dto.AdminObservabilityToolLinks{
			AKHQ:         akhqURL,
			RedisInsight: redisInsightURL,
		},
	}

	if redisObs, err := s.collectRedisObservability(ctx); err != nil {
		resp.RedisError = "获取 Redis 运行状态失败"
	} else {
		resp.Redis = redisObs
	}

	if kafkaObs, err := s.collectKafkaObservability(ctx); err != nil {
		resp.KafkaError = "获取 Kafka 运行状态失败"
	} else {
		resp.Kafka = kafkaObs
	}

	return resp, nil
}

func (s *AdminService) collectRedisObservability(ctx context.Context) (*dto.AdminRedisObservability, error) {
	dbSize, err := redis.Client.DBSize(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("get redis db size failed: %w", err)
	}

	adminStatsReady, err := redis.AdminStatsReady(ctx)
	if err != nil {
		return nil, fmt.Errorf("get redis admin stats status failed: %w", err)
	}

	timeoutQueueSize, err := redis.GetTimeoutQueueSize(ctx)
	if err != nil {
		return nil, fmt.Errorf("get redis timeout queue size failed: %w", err)
	}

	pendingTimeoutCount, err := redis.GetPendingTimeoutCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("get redis pending timeout count failed: %w", err)
	}

	goods, err := s.goodsRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("get goods list failed: %w", err)
	}

	keyspace, err := collectRedisKeyspace(ctx)
	if err != nil {
		return nil, err
	}

	warmupLocks, err := collectRedisWarmupLocks(ctx)
	if err != nil {
		return nil, err
	}

	goodsRuntime, err := collectRedisGoodsRuntime(ctx, goods, s.activitySvc)
	if err != nil {
		return nil, err
	}

	return &dto.AdminRedisObservability{
		DBSize:              dbSize,
		AdminStatsReady:     adminStatsReady,
		TimeoutQueueSize:    timeoutQueueSize,
		PendingTimeoutCount: pendingTimeoutCount,
		Keyspace:            keyspace,
		WarmupLocks:         warmupLocks,
		Goods:               goodsRuntime,
	}, nil
}

func collectRedisKeyspace(ctx context.Context) (dto.AdminRedisKeyspace, error) {
	segmentKeys, err := countKeysByPattern(ctx, "seckill:activity:segment:*")
	if err != nil {
		return dto.AdminRedisKeyspace{}, fmt.Errorf("count redis segment keys failed: %w", err)
	}

	boughtKeys, err := countKeysByPattern(ctx, "seckill:activity:bought:*")
	if err != nil {
		return dto.AdminRedisKeyspace{}, fmt.Errorf("count redis bought keys failed: %w", err)
	}

	processedKeys, err := countKeysByPattern(ctx, "seckill:activity:processed:*")
	if err != nil {
		return dto.AdminRedisKeyspace{}, fmt.Errorf("count redis processed keys failed: %w", err)
	}

	goodsStatusKeys, err := countKeysByPattern(ctx, "seckill:goods:status:*")
	if err != nil {
		return dto.AdminRedisKeyspace{}, fmt.Errorf("count redis goods status keys failed: %w", err)
	}

	userStatusKeys, err := countKeysByPattern(ctx, "seckill:user:status:*")
	if err != nil {
		return dto.AdminRedisKeyspace{}, fmt.Errorf("count redis user status keys failed: %w", err)
	}

	adminStatsKeys, err := countKeysByPattern(ctx, "admin:*")
	if err != nil {
		return dto.AdminRedisKeyspace{}, fmt.Errorf("count redis admin stats keys failed: %w", err)
	}

	totalKeys := segmentKeys + boughtKeys + processedKeys + goodsStatusKeys + userStatusKeys + adminStatsKeys

	return dto.AdminRedisKeyspace{
		TotalKeys:       totalKeys,
		SegmentKeys:     segmentKeys,
		BoughtKeys:      boughtKeys,
		ProcessedKeys:   processedKeys,
		GoodsStatusKeys: goodsStatusKeys,
		UserStatusKeys:  userStatusKeys,
		AdminStatsKeys:  adminStatsKeys,
	}, nil
}

func collectRedisWarmupLocks(ctx context.Context) ([]dto.AdminRedisWarmupLock, error) {
	keys, err := scanKeysByPattern(ctx, warmupLockPrefix+"*")
	if err != nil {
		return nil, fmt.Errorf("scan redis warmup locks failed: %w", err)
	}

	sort.Strings(keys)

	locks := make([]dto.AdminRedisWarmupLock, 0, len(keys))
	for _, key := range keys {
		ttl, err := redis.Client.TTL(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("get redis warmup lock ttl failed: %w", err)
		}

		target := strings.TrimPrefix(key, warmupLockPrefix)
		locks = append(locks, dto.AdminRedisWarmupLock{
			Key:    key,
			Target: target,
			TTLSec: maxInt64(int64(ttl.Seconds()), 0),
		})
	}

	return locks, nil
}

func collectRedisGoodsRuntime(ctx context.Context, goods []model.Goods, activitySvc *ActivityService) ([]dto.AdminRedisGoodsRuntime, error) {
	sort.Slice(goods, func(i, j int) bool {
		return goods[i].ID < goods[j].ID
	})

	result := make([]dto.AdminRedisGoodsRuntime, 0, len(goods))
	for _, item := range goods {
		activityID, activityTitle := defaultActivityForRuntime(ctx, item.ID, activitySvc)
		segmentKeys := make([]string, 0, redis.SegmentCount)
		for segmentID := 0; segmentID < redis.SegmentCount; segmentID++ {
			segmentKeys = append(segmentKeys, redis.SegmentStockKey(activityID, segmentID))
		}

		pipe := redis.Client.Pipeline()
		segmentCmd := pipe.MGet(ctx, segmentKeys...)
		boughtCmd := pipe.HGetAll(ctx, redis.BoughtKey(activityID))
		processedCmd := pipe.HGetAll(ctx, redis.ProcessedKey(activityID))
		metaCmd := pipe.HGetAll(ctx, redis.ActivityMetaKey(activityID))
		_, err := pipe.Exec(ctx)
		if err != nil && err != redisLib.Nil {
			return nil, fmt.Errorf("load redis goods runtime failed: goods_id=%d, err=%w", item.ID, err)
		}

		segmentStocks := make([]dto.AdminRedisSegmentStock, 0, redis.SegmentCount)
		var totalStock int64
		for idx, raw := range segmentCmd.Val() {
			stock := parseRedisInt64(raw)
			totalStock += stock
			segmentStocks = append(segmentStocks, dto.AdminRedisSegmentStock{
				SegmentID: idx,
				Stock:     stock,
			})
		}

		boughtMap := boughtCmd.Val()
		processedMap := processedCmd.Val()
		onSale := false
		meta := metaCmd.Val()
		if len(meta) > 0 {
			onSale = meta["goods_on_sale"] == "1" && meta["warmup_status"] == "1"
		}

		boughtQuantity := sumRedisHashValues(boughtMap)
		processedQuantity := sumRedisHashValues(processedMap)
		pendingQuantity := boughtQuantity - processedQuantity
		if pendingQuantity < 0 {
			pendingQuantity = 0
		}

		result = append(result, dto.AdminRedisGoodsRuntime{
			GoodsID:           item.ID,
			ActivityID:        activityID,
			GoodsName:         item.ProductName,
			ActivityTitle:     activityTitle,
			OnSale:            onSale,
			TotalStock:        totalStock,
			BoughtUsers:       int64(len(boughtMap)),
			BoughtQuantity:    boughtQuantity,
			ProcessedUsers:    int64(len(processedMap)),
			ProcessedQuantity: processedQuantity,
			PendingQuantity:   pendingQuantity,
			SegmentStocks:     segmentStocks,
		})
	}

	return result, nil
}

func defaultActivityForRuntime(ctx context.Context, goodsID uint64, activitySvc *ActivityService) (uint64, string) {
	if activitySvc != nil && activitySvc.activityRepo != nil {
		if activity, err := activitySvc.activityRepo.FindDefaultByGoodsID(goodsID); err == nil {
			return activity.ID, activity.Title
		}
	}
	if activityID, ok, err := redis.GetDefaultActivityID(ctx, goodsID); err == nil && ok {
		return activityID, ""
	}
	return goodsID, ""
}

func (s *AdminService) collectKafkaObservability(ctx context.Context) (*dto.AdminKafkaObservability, error) {
	brokers := append([]string(nil), s.seckillSvc.cfg.Kafka.Brokers...)
	topic := s.seckillSvc.cfg.Kafka.Topic
	group := s.seckillSvc.cfg.Kafka.Group
	dlqTopic := topic + "_dlq"

	client := &kafkaLib.Client{
		Addr:    kafkaLib.TCP(brokers...),
		Timeout: 5 * time.Second,
	}

	metadataResp, err := client.Metadata(ctx, &kafkaLib.MetadataRequest{
		Topics: []string{topic, dlqTopic},
	})
	if err != nil {
		return nil, fmt.Errorf("get kafka metadata failed: %w", err)
	}

	mainTopicMeta, ok := findKafkaTopicMetadata(metadataResp.Topics, topic)
	if !ok {
		return nil, fmt.Errorf("kafka topic metadata not found: %s", topic)
	}
	if mainTopicMeta.Error != nil {
		return nil, fmt.Errorf("kafka topic metadata error: %w", mainTopicMeta.Error)
	}

	sort.Slice(mainTopicMeta.Partitions, func(i, j int) bool {
		return mainTopicMeta.Partitions[i].ID < mainTopicMeta.Partitions[j].ID
	})

	offsetReqs := map[string][]kafkaLib.OffsetRequest{
		topic: make([]kafkaLib.OffsetRequest, 0, len(mainTopicMeta.Partitions)*2),
	}
	partitionIDs := make([]int, 0, len(mainTopicMeta.Partitions))
	for _, partition := range mainTopicMeta.Partitions {
		partitionIDs = append(partitionIDs, partition.ID)
		offsetReqs[topic] = append(offsetReqs[topic], kafkaLib.FirstOffsetOf(partition.ID), kafkaLib.LastOffsetOf(partition.ID))
	}

	if dlqMeta, ok := findKafkaTopicMetadata(metadataResp.Topics, dlqTopic); ok && dlqMeta.Error == nil {
		offsetReqs[dlqTopic] = make([]kafkaLib.OffsetRequest, 0, len(dlqMeta.Partitions))
		for _, partition := range dlqMeta.Partitions {
			offsetReqs[dlqTopic] = append(offsetReqs[dlqTopic], kafkaLib.LastOffsetOf(partition.ID))
		}
	}

	offsetResp, err := client.ListOffsets(ctx, &kafkaLib.ListOffsetsRequest{Topics: offsetReqs})
	if err != nil {
		return nil, fmt.Errorf("get kafka offsets failed: %w", err)
	}

	partitionOffsets := make(map[int]kafkaLib.PartitionOffsets, len(mainTopicMeta.Partitions))
	for _, item := range offsetResp.Topics[topic] {
		partitionOffsets[item.Partition] = item
	}

	memberByPartition := make(map[int]dto.AdminKafkaMemberRuntime, len(mainTopicMeta.Partitions))
	groupState := "unknown"
	members := make([]dto.AdminKafkaMemberRuntime, 0)
	committedOffsets := make(map[int]int64, len(mainTopicMeta.Partitions))

	if group != "" {
		if coordinator, err := client.FindCoordinator(ctx, &kafkaLib.FindCoordinatorRequest{
			Key:     group,
			KeyType: kafkaLib.CoordinatorKeyTypeConsumer,
		}); err == nil && coordinator != nil && coordinator.Coordinator != nil {
			coordinatorAddr := kafkaLib.TCP(net.JoinHostPort(coordinator.Coordinator.Host, strconv.Itoa(coordinator.Coordinator.Port)))

			if offsetFetchResp, err := client.OffsetFetch(ctx, &kafkaLib.OffsetFetchRequest{
				Addr:    coordinatorAddr,
				GroupID: group,
				Topics: map[string][]int{
					topic: partitionIDs,
				},
			}); err == nil {
				for _, item := range offsetFetchResp.Topics[topic] {
					if item.Error == nil {
						committedOffsets[item.Partition] = item.CommittedOffset
					}
				}
			}

			if describeResp, err := client.DescribeGroups(ctx, &kafkaLib.DescribeGroupsRequest{
				Addr:     coordinatorAddr,
				GroupIDs: []string{group},
			}); err == nil && len(describeResp.Groups) > 0 {
				groupState = describeResp.Groups[0].GroupState
				members = make([]dto.AdminKafkaMemberRuntime, 0, len(describeResp.Groups[0].Members))

				for _, member := range describeResp.Groups[0].Members {
					partitions := assignedPartitionsForTopic(member.MemberAssignments.Topics, topic)
					sort.Ints(partitions)

					memberRuntime := dto.AdminKafkaMemberRuntime{
						MemberID:   member.MemberID,
						ClientID:   member.ClientID,
						ClientHost: member.ClientHost,
						Partitions: partitions,
					}
					members = append(members, memberRuntime)

					for _, partitionID := range partitions {
						memberByPartition[partitionID] = memberRuntime
					}
				}
			}
		}
	}

	partitions := make([]dto.AdminKafkaPartitionRuntime, 0, len(mainTopicMeta.Partitions))
	var totalLatestOffset int64
	var totalCommittedOffset int64
	var totalLag int64

	for _, partition := range mainTopicMeta.Partitions {
		offsets := partitionOffsets[partition.ID]
		committedOffset := committedOffsets[partition.ID]
		if committedOffset == 0 {
			if _, ok := committedOffsets[partition.ID]; !ok {
				committedOffset = -1
			}
		}

		lag := offsets.LastOffset
		if committedOffset >= 0 {
			lag = offsets.LastOffset - committedOffset
		}
		if lag < 0 {
			lag = 0
		}

		member := memberByPartition[partition.ID]
		partitions = append(partitions, dto.AdminKafkaPartitionRuntime{
			Partition:       partition.ID,
			Leader:          formatKafkaBroker(partition.Leader),
			EarliestOffset:  offsets.FirstOffset,
			LatestOffset:    offsets.LastOffset,
			CommittedOffset: committedOffset,
			Lag:             lag,
			MemberID:        member.MemberID,
			ClientID:        member.ClientID,
			ClientHost:      member.ClientHost,
		})

		totalLatestOffset += maxInt64(offsets.LastOffset, 0)
		if committedOffset >= 0 {
			totalCommittedOffset += committedOffset
		}
		totalLag += lag
	}

	var dlqDepth int64
	for _, item := range offsetResp.Topics[dlqTopic] {
		dlqDepth += maxInt64(item.LastOffset, 0)
	}

	return &dto.AdminKafkaObservability{
		Brokers:              brokers,
		Topic:                topic,
		Group:                group,
		GroupState:           groupState,
		PartitionCount:       len(partitions),
		ActiveMemberCount:    len(members),
		TotalLatestOffset:    totalLatestOffset,
		TotalCommittedOffset: totalCommittedOffset,
		TotalLag:             totalLag,
		DLQTopic:             dlqTopic,
		DLQDepth:             dlqDepth,
		Members:              members,
		Partitions:           partitions,
	}, nil
}

func countKeysByPattern(ctx context.Context, pattern string) (int64, error) {
	keys, err := scanKeysByPattern(ctx, pattern)
	if err != nil {
		return 0, err
	}
	return int64(len(keys)), nil
}

func scanKeysByPattern(ctx context.Context, pattern string) ([]string, error) {
	var (
		cursor uint64
		keys   []string
	)

	for {
		batch, nextCursor, err := redis.Client.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return keys, nil
}

func parseRedisInt64(raw interface{}) int64 {
	switch value := raw.(type) {
	case nil:
		return 0
	case string:
		n, _ := strconv.ParseInt(value, 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(string(value), 10, 64)
		return n
	case int64:
		return value
	case int:
		return int64(value)
	default:
		n, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		return n
	}
}

func sumRedisHashValues(values map[string]string) int64 {
	var total int64
	for _, raw := range values {
		n, _ := strconv.ParseInt(raw, 10, 64)
		total += n
	}
	return total
}

func findKafkaTopicMetadata(topics []kafkaLib.Topic, name string) (kafkaLib.Topic, bool) {
	for _, item := range topics {
		if item.Name == name {
			return item, true
		}
	}
	return kafkaLib.Topic{}, false
}

func assignedPartitionsForTopic(topics []kafkaLib.GroupMemberTopic, targetTopic string) []int {
	partitions := make([]int, 0)
	for _, item := range topics {
		if item.Topic != targetTopic {
			continue
		}
		partitions = append(partitions, item.Partitions...)
	}
	return partitions
}

func formatKafkaBroker(broker kafkaLib.Broker) string {
	if broker.Host == "" || broker.Port == 0 {
		return "-"
	}
	return net.JoinHostPort(broker.Host, strconv.Itoa(broker.Port))
}

func maxInt64(value, min int64) int64 {
	if value < min {
		return min
	}
	return value
}
