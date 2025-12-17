# Design Document

## Overview

本设计将秒杀系统从 RocketMQ 迁移到 Kafka，并实现基于 Redis ZSET + MySQL 兜底的订单超时机制。

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                           秒杀系统架构                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────────────┐  │
│  │  User   │───▶│   API   │───▶│  Redis  │───▶│  Kafka Producer │  │
│  └─────────┘    └─────────┘    │ (库存)  │    └────────┬────────┘  │
│                                └─────────┘             │            │
│                                                        ▼            │
│                                              ┌─────────────────┐    │
│                                              │     Kafka       │    │
│                                              │  (seckill topic)│    │
│                                              └────────┬────────┘    │
│                                                       │             │
│  ┌──────────────────────────────────────────────────┐ │             │
│  │                    Worker                         │ │             │
│  │  ┌────────────────┐  ┌────────────────────────┐  │◀┘             │
│  │  │ Kafka Consumer │  │   Timeout Workers      │  │               │
│  │  │ (订单创建)      │  │  ┌─────────────────┐  │  │               │
│  │  └───────┬────────┘  │  │ Redis ZSET 扫描  │  │  │               │
│  │          │           │  │ (每秒)           │  │  │               │
│  │          ▼           │  └─────────────────┘  │  │               │
│  │  ┌────────────────┐  │  ┌─────────────────┐  │  │               │
│  │  │ MySQL 批量写入  │  │  │ MySQL 兜底扫描  │  │  │               │
│  │  │ + Redis ZSET   │  │  │ (每5分钟)       │  │  │               │
│  │  └────────────────┘  │  └─────────────────┘  │  │               │
│  └──────────────────────────────────────────────────┘               │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Components and Interfaces

### 1. Kafka Producer (`pkg/mq/kafka.go`)

```go
// Producer 接口
type Producer interface {
    SendMessage(ctx context.Context, topic string, key, value []byte) error
    SendBatch(ctx context.Context, topic string, messages []Message) error
    Close() error
}

// Message 消息结构
type Message struct {
    Key   []byte
    Value []byte
}
```

### 2. Kafka Consumer (`internal/worker/kafka_consumer.go`)

```go
// Consumer 接口
type Consumer interface {
    Start() error
    Stop() error
}

// 消费配置
type ConsumerConfig struct {
    Brokers       []string
    Topic         string
    GroupID       string
    MinBytes      int  // 最小拉取字节数
    MaxBytes      int  // 最大拉取字节数
    CommitInterval time.Duration
}
```

### 3. 订单超时队列 (`pkg/redis/delay_queue.go`)

```go
// DelayQueue 延迟队列接口
type DelayQueue interface {
    Add(ctx context.Context, orderID uint64, expireAt time.Time) error
    PopExpired(ctx context.Context, limit int64) ([]uint64, error)
    Remove(ctx context.Context, orderID uint64) error
}
```

### 4. 超时 Worker (`internal/worker/timeout_worker.go`)

```go
// TimeoutWorker 超时处理 Worker
type TimeoutWorker struct {
    redisScanner   *RedisTimeoutScanner   // 每秒扫描
    mysqlScanner   *MySQLTimeoutScanner   // 每5分钟扫描
}
```

## Data Models

### Kafka 消息格式

```go
// SeckillMessage 秒杀消息 (与原有保持一致)
type SeckillMessage struct {
    UserID      uint64 `json:"user_id"`
    GoodsID     uint64 `json:"goods_id"`
    SegmentID   int    `json:"segment_id"`
    RequestTime int64  `json:"request_time"`
    CreateTime  int64  `json:"create_time"`
}
```

### Redis ZSET 结构

```
Key: seckill:order:timeout
Score: 超时时间戳 (Unix seconds)
Member: orderID (string)
```

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system.*

### Property 1: Message Delivery Guarantee
*For any* seckill message sent to Kafka, if the send returns success, the message SHALL be persisted and eventually consumed by the Worker.
**Validates: Requirements 1.2, 2.2**

### Property 2: Order Timeout Atomicity
*For any* order in the timeout ZSET, the pop operation SHALL atomically remove and return the order, preventing duplicate processing.
**Validates: Requirements 3.2**

### Property 3: Stock Consistency
*For any* cancelled order, the stock SHALL be restored exactly once to both Redis and MySQL.
**Validates: Requirements 3.4, 4.2**

### Property 4: Fallback Coverage
*For any* order that expires, if Redis ZSET fails to process it, the MySQL fallback scanner SHALL eventually cancel it.
**Validates: Requirements 4.1, 4.2**

## Error Handling

| 场景 | 处理策略 |
|------|----------|
| Kafka 发送失败 | 指数退避重试 3 次，失败后返还 Redis 库存 |
| Kafka 消费失败 | 不提交 offset，消息会被重新消费 |
| Redis ZSET 操作失败 | 记录日志，依赖 MySQL 兜底 |
| MySQL 写入失败 | 清除 Redis 标记，允许用户重试 |

## Testing Strategy

### Unit Tests
- Kafka Producer/Consumer 连接和消息收发
- Redis ZSET 延迟队列操作
- 订单取消逻辑

### Property-Based Tests
使用 `github.com/leanovate/gopter` 进行属性测试：
- 消息序列化/反序列化 round-trip
- 并发订单超时处理不重复

### Integration Tests
- 完整秒杀流程：请求 → Kafka → Worker → MySQL
- 订单超时流程：创建 → 等待 → 自动取消

## Configuration

```yaml
kafka:
  brokers:
    - "kafka:9092"
  topic: "seckill_orders"
  group: "seckill_consumer_group"
  
timeout:
  order_timeout_seconds: 60
  redis_scan_interval: "1s"
  mysql_scan_interval: "5m"
```
