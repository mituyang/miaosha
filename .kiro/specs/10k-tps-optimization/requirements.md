# Requirements Document

## Introduction

将秒杀系统的吞吐量从当前水平优化到 10,000 TPS（每秒处理 1 万次秒杀请求）。优化涵盖 Redis 分段策略、Kafka 配置、连接池调优、Worker 并行消费等多个层面。

## Glossary

- **TPS**: Transactions Per Second，每秒事务处理数
- **Segment**: Redis 库存分段，用于分散热点 key 压力
- **Partition**: Kafka 分区，支持并行消费
- **Consumer Group**: Kafka 消费者组，多个 Consumer 共同消费一个 Topic
- **Batch**: 批量处理，减少网络往返次数
- **Connection Pool**: 连接池，复用数据库/缓存连接

## Requirements

### Requirement 1

**User Story:** As a system operator, I want to increase Redis segment count, so that hot key contention is reduced under high concurrency.

#### Acceptance Criteria

1. WHEN the system initializes stock THEN the system SHALL distribute stock across 32 segments instead of 10
2. WHEN a seckill request arrives THEN the system SHALL randomly select a starting segment to distribute load evenly
3. WHEN stock is restored (order cancelled) THEN the system SHALL return stock to the correct segment

### Requirement 2

**User Story:** As a system operator, I want to increase Kafka partitions, so that message throughput and parallel consumption are improved.

#### Acceptance Criteria

1. WHEN docker-compose starts THEN Kafka SHALL create the seckill topic with 32 partitions
2. WHEN messages are produced THEN the Producer SHALL use goods_id as partition key for ordering guarantee
3. WHEN multiple Workers consume THEN the system SHALL distribute partitions evenly across consumers

### Requirement 3

**User Story:** As a system operator, I want to optimize Kafka Producer configuration, so that message sending throughput is maximized.

#### Acceptance Criteria

1. WHEN the Producer sends messages THEN the system SHALL use a buffer size of at least 200,000 messages
2. WHEN batching messages THEN the system SHALL use batch size of 1000 and batch timeout of 5ms
3. WHEN the Producer starts THEN the system SHALL launch 100 sender goroutines for parallel sending

### Requirement 4

**User Story:** As a system operator, I want to scale Worker consumption capacity, so that order creation keeps pace with incoming requests.

#### Acceptance Criteria

1. WHEN the Worker starts THEN the system SHALL launch multiple consumer goroutines (one per partition)
2. WHEN processing messages THEN each consumer goroutine SHALL process messages independently
3. WHEN batch writing to MySQL THEN the system SHALL use batch size of 2000 orders per flush

### Requirement 5

**User Story:** As a system operator, I want to optimize connection pool sizes, so that database and cache connections are not bottlenecks.

#### Acceptance Criteria

1. WHEN the system starts THEN MySQL connection pool SHALL have at least 2000 max connections
2. WHEN the system starts THEN Redis connection pool SHALL have at least 2000 connections
3. WHEN the system starts THEN HTTP client SHALL support at least 10000 concurrent connections

### Requirement 6

**User Story:** As a system operator, I want to optimize Kafka broker configuration, so that the broker can handle high throughput.

#### Acceptance Criteria

1. WHEN Kafka starts THEN the broker SHALL allocate at least 4GB heap memory
2. WHEN Kafka starts THEN the broker SHALL configure appropriate network and IO thread counts
3. WHEN Kafka starts THEN the broker SHALL set socket buffer sizes for high throughput

