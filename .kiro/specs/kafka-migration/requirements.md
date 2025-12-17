# Requirements Document

## Introduction

将秒杀系统的消息队列从 RocketMQ 迁移到 Kafka，同时将订单超时机制从 RocketMQ 定时消息改为 Redis ZSET + MySQL 兜底扫描的双重保障方案。

## Glossary

- **Kafka**: Apache Kafka 分布式消息队列
- **ZooKeeper**: Kafka 集群协调服务
- **ZSET**: Redis 有序集合，用于实现延迟队列
- **Consumer Group**: Kafka 消费者组，支持分区并行消费
- **Partition**: Kafka 分区，消息的物理存储单元

## Requirements

### Requirement 1

**User Story:** As a developer, I want to replace RocketMQ with Kafka, so that I can leverage Kafka's higher throughput and simpler ecosystem.

#### Acceptance Criteria

1. WHEN the system starts THEN the Kafka Producer SHALL connect to the Kafka cluster and be ready to send messages
2. WHEN a seckill request succeeds THEN the system SHALL send a message to Kafka topic within 10ms
3. WHEN Kafka is unavailable THEN the system SHALL retry with exponential backoff and return error after max retries
4. WHEN the system shuts down THEN the Kafka Producer SHALL flush pending messages and close gracefully

### Requirement 2

**User Story:** As a developer, I want the Worker to consume messages from Kafka, so that orders can be created asynchronously.

#### Acceptance Criteria

1. WHEN the Worker starts THEN the Kafka Consumer SHALL join the consumer group and begin consuming messages
2. WHEN a message is consumed THEN the Worker SHALL process it and commit the offset only after successful processing
3. WHEN message processing fails THEN the Worker SHALL retry the message before committing
4. WHEN multiple Workers are deployed THEN the system SHALL distribute partitions evenly across consumers

### Requirement 3

**User Story:** As a developer, I want to implement order timeout using Redis ZSET, so that unpaid orders are cancelled automatically.

#### Acceptance Criteria

1. WHEN an order is created THEN the system SHALL add the order ID to Redis ZSET with expiration timestamp as score
2. WHEN the timeout worker scans THEN the system SHALL atomically fetch and remove expired orders from ZSET
3. WHEN an order is paid THEN the system SHALL remove the order from the timeout ZSET
4. WHEN an order is cancelled THEN the system SHALL restore stock to Redis and MySQL

### Requirement 4

**User Story:** As a developer, I want MySQL fallback scanning for order timeout, so that no orders are missed if Redis fails.

#### Acceptance Criteria

1. WHEN the fallback scanner runs (every 5 minutes) THEN the system SHALL query orders with status=0 and create_time older than timeout threshold
2. WHEN expired orders are found in MySQL THEN the system SHALL cancel them and restore stock
3. WHEN cancelling orders THEN the system SHALL use CAS update to prevent duplicate cancellation

### Requirement 5

**User Story:** As a developer, I want to update Docker Compose configuration, so that the system uses Kafka instead of RocketMQ.

#### Acceptance Criteria

1. WHEN docker-compose up is executed THEN ZooKeeper SHALL start before Kafka
2. WHEN docker-compose up is executed THEN Kafka SHALL be accessible on port 9092
3. WHEN the configuration is applied THEN RocketMQ services (namesrv, broker) SHALL be removed
