# Design Document

## Overview

本设计文档描述如何将秒杀系统优化到 10,000 TPS。优化策略包括增加 Redis 分段数、扩展 Kafka 分区、优化 Producer/Consumer 配置、调整连接池大小等。

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        10K TPS 优化架构                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────┐    ┌─────────────────┐    ┌──────────────────────────────────┐ │
│  │  Users  │───▶│   API Server    │───▶│         Redis Cluster            │ │
│  │ 10K/s   │    │  (Pool: 2000)   │    │  32 Segments per Goods           │ │
│  └─────────┘    └────────┬────────┘    │  Pool: 2000 connections          │ │
│                          │             └──────────────────────────────────┘ │
│                          │                                                  │
│                          ▼                                                  │
│              ┌───────────────────────┐                                      │
│              │    Kafka Producer     │                                      │
│              │  Buffer: 200K msgs    │                                      │
│              │  Senders: 100         │                                      │
│              │  Batch: 1000/5ms      │                                      │
│              └───────────┬───────────┘                                      │
│                          │                                                  │
│                          ▼                                                  │
│              ┌───────────────────────┐                                      │
│              │   Kafka Broker        │                                      │
│              │  32 Partitions        │                                      │
│              │  Heap: 4GB            │                                      │
│              └───────────┬───────────┘                                      │
│                          │                                                  │
│         ┌────────────────┼────────────────┐                                 │
│         ▼                ▼                ▼                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                          │
│  │  Consumer   │  │  Consumer   │  │  Consumer   │  ... (32 goroutines)     │
│  │  Partition0 │  │  Partition1 │  │  Partition2 │                          │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                          │
│         │                │                │                                 │
│         └────────────────┼────────────────┘                                 │
│                          ▼                                                  │
│              ┌───────────────────────┐                                      │
│              │   Batch Writer        │                                      │
│              │  Batch: 2000 orders   │                                      │
│              │  Flush: 100ms         │                                      │
│              └───────────┬───────────┘                                      │
│                          │                                                  │
│                          ▼                                                  │
│              ┌───────────────────────┐                                      │
│              │   MySQL               │                                      │
│              │  Pool: 2000 conns     │                                      │
│              └───────────────────────┘                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Components and Interfaces

### 1. Redis 分段配置 (`pkg/redis/keys.go`)

```go
const (
    SegmentCount = 32  // 从 10 增加到 32
)
```

### 2. Kafka Producer 配置 (`pkg/mq/kafka.go`)

```go
const (
    kafkaBatchSize    = 1000                  // 从 500 增加到 1000
    kafkaBatchTimeout = 5 * time.Millisecond  // 从 10ms 减少到 5ms
    kafkaBufferSize   = 200000                // 从 100000 增加到 200000
    kafkaSenderCount  = 100                   // 从 50 增加到 100
)
```

### 3. Kafka Consumer 配置 (`internal/worker/kafka_consumer.go`)

```go
const (
    kafkaConsumerCount      = 32                    // 并行消费协程数
    kafkaBatchFlushInterval = 100 * time.Millisecond
    kafkaBatchSize          = 2000                  // 从 1000 增加到 2000
    kafkaBatchQueueSize     = 50000                 // 从 10000 增加到 50000
)
```

### 4. 连接池配置 (`configs/config.yaml`)

```yaml
mysql:
  max_open_conns: 2000  # 从 1000 增加到 2000
  max_idle_conns: 2000

redis:
  pool_size: 2000       # 从 1000 增加到 2000
```

### 5. Kafka Broker 配置 (`docker-compose.yml`)

```yaml
kafka:
  environment:
    KAFKA_NUM_PARTITIONS: 32
    KAFKA_HEAP_OPTS: "-Xmx4g -Xms4g"
    KAFKA_NUM_NETWORK_THREADS: 8
    KAFKA_NUM_IO_THREADS: 16
    KAFKA_SOCKET_SEND_BUFFER_BYTES: 1048576
    KAFKA_SOCKET_RECEIVE_BUFFER_BYTES: 1048576
```

## Data Models

无新增数据模型，仅配置参数调整。

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system-essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Stock Distribution Consistency
*For any* goods with initial stock S, after initialization, the sum of all 32 segment stocks SHALL equal S.
**Validates: Requirements 1.1**

### Property 2: Stock Restoration Integrity
*For any* successful seckill that deducts from segment X, if the order is cancelled, restoring stock to segment X SHALL preserve total stock count.
**Validates: Requirements 1.3**

### Property 3: Partition Key Consistency
*For any* two messages with the same goods_id, both messages SHALL be routed to the same Kafka partition.
**Validates: Requirements 2.2**

## Error Handling

| 场景 | 处理策略 |
|------|----------|
| Kafka Buffer 满 | 降级为同步发送，保证消息不丢失 |
| Consumer 处理失败 | 不提交 offset，消息重新消费 |
| MySQL 连接池耗尽 | 等待可用连接，记录告警日志 |
| Redis 连接池耗尽 | 返回错误，允许用户重试 |

## Testing Strategy

### Unit Tests
- 验证 Redis 分段初始化正确分配库存
- 验证 Kafka Producer 配置参数正确
- 验证连接池配置生效

### Property-Based Tests
使用 `github.com/leanovate/gopter` 进行属性测试：

1. **Stock Distribution Property**: 生成随机库存数，验证分段后总和不变
2. **Stock Restoration Property**: 随机扣减和恢复操作，验证库存一致性
3. **Partition Key Property**: 生成随机 goods_id，验证相同 ID 路由到相同分区

### Integration Tests
- 压测验证：2000 并发，40 秒持续压测，验证 TPS 达到 10000
- 验证消息不丢失：发送数 = 消费数

## Configuration Changes Summary

| 组件 | 参数 | 原值 | 新值 |
|------|------|------|------|
| Redis | SegmentCount | 10 | 32 |
| Kafka Producer | kafkaBatchSize | 500 | 1000 |
| Kafka Producer | kafkaBatchTimeout | 10ms | 5ms |
| Kafka Producer | kafkaBufferSize | 100000 | 200000 |
| Kafka Producer | kafkaSenderCount | 50 | 100 |
| Kafka Consumer | kafkaBatchQueueSize | 10000 | 50000 |
| Kafka Consumer | batch size | 1000 | 2000 |
| Kafka Broker | KAFKA_NUM_PARTITIONS | 8 | 32 |
| Kafka Broker | KAFKA_HEAP_OPTS | 2g | 4g |
| MySQL | max_open_conns | 1000 | 2000 |
| Redis | pool_size | 1000 | 2000 |

