# Implementation Plan

## Phase 1: Redis 分段优化

- [x] 1. Increase Redis segment count
  - [x] 1.1 Update SegmentCount constant in `pkg/redis/keys.go`
    - Change from 10 to 32
    - _Requirements: 1.1_
  - [x] 1.2 Write property test for stock distribution
    - **Property 1: Stock Distribution Consistency**
    - **Validates: Requirements 1.1**
  - [x] 1.3 Write property test for stock restoration
    - **Property 2: Stock Restoration Integrity**
    - **Validates: Requirements 1.3**

## Phase 2: Kafka Producer 优化

- [x] 2. Optimize Kafka Producer configuration
  - [x] 2.1 Update Producer constants in `pkg/mq/kafka.go`
    - kafkaBatchSize: 500 → 1000
    - kafkaBatchTimeout: 10ms → 5ms
    - kafkaBufferSize: 100000 → 200000
    - kafkaSenderCount: 50 → 100
    - _Requirements: 3.1, 3.2, 3.3_
  - [x] 2.2 Write property test for partition key consistency
    - **Property 3: Partition Key Consistency**
    - **Validates: Requirements 2.2**

## Phase 3: Kafka Consumer 优化

- [x] 3. Optimize Kafka Consumer configuration
  - [x] 3.1 Update Consumer constants in `internal/worker/kafka_consumer.go`
    - kafkaBatchQueueSize: 10000 → 50000
    - Batch flush size: 1000 → 2000
    - _Requirements: 4.1, 4.3_
  - [x] 3.2 Implement parallel consumer goroutines
    - Launch multiple consumer goroutines for parallel processing
    - _Requirements: 4.1, 4.2_

## Phase 4: 连接池优化

- [x] 4. Optimize connection pool sizes
  - [x] 4.1 Update MySQL pool in `configs/config.yaml`
    - max_open_conns: 1000 → 2000
    - max_idle_conns: 1000 → 2000
    - _Requirements: 5.1_
  - [x] 4.2 Update MySQL pool in `configs/config.docker.yaml`
    - Same changes as config.yaml
    - _Requirements: 5.1_
  - [x] 4.3 Update Redis pool in both config files
    - pool_size: 1000 → 2000
    - _Requirements: 5.2_

## Phase 5: Kafka Broker 优化

- [x] 5. Optimize Kafka broker configuration in docker-compose.yml
  - [x] 5.1 Increase partition count
    - KAFKA_NUM_PARTITIONS: 8 → 32
    - _Requirements: 2.1_
  - [x] 5.2 Increase heap memory
    - KAFKA_HEAP_OPTS: "-Xmx4g -Xms4g"
    - _Requirements: 6.1_
  - [x] 5.3 Add network and IO thread configuration
    - KAFKA_NUM_NETWORK_THREADS: 8
    - KAFKA_NUM_IO_THREADS: 16
    - _Requirements: 6.2_
  - [x] 5.4 Add socket buffer configuration
    - KAFKA_SOCKET_SEND_BUFFER_BYTES: 1048576
    - KAFKA_SOCKET_RECEIVE_BUFFER_BYTES: 1048576
    - _Requirements: 6.3_

## Phase 6: 验证

- [x] 6. Checkpoint - Verify all changes
  - Ensure all tests pass, ask the user if questions arise.

- [x] 7. Performance validation
  - [x] 7.1 Run benchmark test with 2000 concurrency for 40 seconds
    - Verify TPS reaches 10000
    - _Requirements: 1.1, 2.1, 3.1, 4.1, 5.1, 6.1_

