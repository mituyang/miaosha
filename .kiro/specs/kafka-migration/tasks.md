# Implementation Plan

## Phase 1: Infrastructure

- [x] 1. Update Docker Compose configuration
  - [x] 1.1 Add ZooKeeper service
    - Image: `confluentinc/cp-zookeeper:7.5.0`
    - Port: 2181
    - _Requirements: 5.1_
  - [x] 1.2 Add Kafka service
    - Image: `confluentinc/cp-kafka:7.5.0`
    - Port: 9092
    - Depends on ZooKeeper
    - _Requirements: 5.2_
  - [x] 1.3 Remove RocketMQ services (namesrv, broker)
    - _Requirements: 5.3_
  - [x] 1.4 Update backend and worker dependencies
    - _Requirements: 5.1, 5.2_

## Phase 2: Kafka Integration

- [x] 2. Implement Kafka Producer
  - [x] 2.1 Create `pkg/mq/kafka.go` with Producer implementation
    - Use `segmentio/kafka-go`
    - Implement batch sending with buffer
    - _Requirements: 1.1, 1.2_
  - [x] 2.2 Implement retry logic with exponential backoff
    - _Requirements: 1.3_
  - [x] 2.3 Implement graceful shutdown
    - _Requirements: 1.4_
  - [ ]* 2.4 Write unit tests for Kafka Producer
    - _Requirements: 1.1, 1.2, 1.3_

- [x] 3. Implement Kafka Consumer
  - [x] 3.1 Create `internal/worker/kafka_consumer.go`
    - Consumer group support
    - Manual offset commit after processing
    - _Requirements: 2.1, 2.2_
  - [x] 3.2 Implement message processing with retry
    - _Requirements: 2.3_
  - [x] 3.3 Update Worker to use Kafka Consumer
    - Replace RocketMQ consumer
    - _Requirements: 2.1, 2.4_
  - [ ]* 3.4 Write unit tests for Kafka Consumer
    - _Requirements: 2.1, 2.2_

- [x] 4. Checkpoint - Kafka integration
  - Ensure all tests pass, ask the user if questions arise.

## Phase 3: Order Timeout (Redis ZSET)

- [x] 5. Implement Redis Delay Queue
  - [x] 5.1 Create `pkg/redis/delay_queue.go`
    - Add order to ZSET with expiration score
    - Atomic pop expired orders (Lua script)
    - Remove order from ZSET
    - _Requirements: 3.1, 3.2, 3.3_
  - [x] 5.2 Add Lua script for atomic pop operation
    - _Requirements: 3.2_
  - [ ]* 5.3 Write unit tests for delay queue
    - **Property 2: Order Timeout Atomicity**
    - **Validates: Requirements 3.2**

- [x] 6. Implement Redis Timeout Scanner
  - [x] 6.1 Create `internal/worker/redis_timeout_scanner.go`
    - Scan every 1 second
    - Process expired orders in batches
    - _Requirements: 3.2, 3.4_
  - [x] 6.2 Implement order cancellation logic
    - CAS update order status
    - Restore Redis and MySQL stock
    - Clear user marks
    - _Requirements: 3.4_
  - [ ]* 6.3 Write unit tests for timeout scanner
    - **Property 3: Stock Consistency**
    - **Validates: Requirements 3.4**

## Phase 4: MySQL Fallback Scanner

- [x] 7. Implement MySQL Fallback Scanner
  - [x] 7.1 Create `internal/worker/mysql_timeout_scanner.go`
    - Scan every 5 minutes
    - Query: `status=0 AND create_time < NOW() - INTERVAL 60 SECOND`
    - _Requirements: 4.1_
  - [x] 7.2 Implement batch cancellation with CAS
    - _Requirements: 4.2, 4.3_
  - [ ]* 7.3 Write unit tests for MySQL scanner
    - **Property 4: Fallback Coverage**
    - **Validates: Requirements 4.1, 4.2**

- [x] 8. Checkpoint - Timeout mechanism
  - Ensure all tests pass, ask the user if questions arise.

## Phase 5: Integration

- [x] 9. Update Order Creation Flow
  - [x] 9.1 Modify `flushGoodsBatch` to add orders to Redis ZSET
    - Replace RocketMQ timer message with ZSET add
    - _Requirements: 3.1_
  - [x] 9.2 Update order payment to remove from ZSET
    - _Requirements: 3.3_

- [x] 10. Update Configuration
  - [x] 10.1 Add Kafka config to `config.yaml`
    - Brokers, topic, group
    - _Requirements: 1.1, 2.1_
  - [x] 10.2 Add timeout config
    - Redis scan interval, MySQL scan interval
    - _Requirements: 3.2, 4.1_
  - [x] 10.3 Update `internal/config/config.go`
    - _Requirements: 1.1, 2.1_

- [x] 11. Update API Server
  - [x] 11.1 Replace RocketMQ Producer with Kafka Producer in `main.go`
    - _Requirements: 1.1_
  - [x] 11.2 Update `seckill_service.go` to use Kafka
    - _Requirements: 1.2_

- [x] 12. Update Worker
  - [x] 12.1 Replace RocketMQ Consumer with Kafka Consumer
    - _Requirements: 2.1_
  - [x] 12.2 Add Redis Timeout Scanner
    - _Requirements: 3.2_
  - [x] 12.3 Add MySQL Fallback Scanner
    - _Requirements: 4.1_
  - [x] 12.4 Update Worker main.go
    - _Requirements: 2.1, 3.2, 4.1_

## Phase 6: Cleanup

- [x] 13. Remove RocketMQ Code
  - [x] 13.1 Delete `pkg/mq/rocketmq.go`
  - [x] 13.2 Remove RocketMQ dependencies from `go.mod`
  - [x] 13.3 Delete `backend/configs/broker.conf`

- [x] 14. Final Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

## Phase 7: Documentation

- [ ] 15. Update Documentation
  - [ ] 15.1 Create README.md with project overview and new architecture
    - Include system architecture diagram
    - Document Kafka + Redis ZSET timeout mechanism
    - _Requirements: 1.1, 2.1, 3.1, 4.1, 5.1, 5.2_
  - [ ] 15.2 Add deployment and setup instructions
    - Docker Compose usage
    - Configuration options
    - _Requirements: 5.1, 5.2_
