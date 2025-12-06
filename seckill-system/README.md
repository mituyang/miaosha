# 电商秒杀系统

基于 Go 实现的高并发秒杀系统，采用微服务架构。

## 架构设计

```
用户请求 -> [Gin API Gateway] -> [gRPC 秒杀服务] -> [Redis Lua 原子扣减] -> [Kafka] -> [消费者] -> [MySQL]
```

## 技术栈

- **Web 框架**: Gin (HTTP API Gateway)
- **微服务通信**: gRPC + Protobuf
- **缓存**: Redis (Lua 脚本原子操作)
- **消息队列**: Kafka (异步解耦)
- **数据库**: MySQL + GORM

## 项目结构

```
seckill-system/
├── api/proto/           # Protobuf 定义
├── cmd/
│   ├── gateway/         # HTTP API 网关
│   ├── seckill/         # gRPC 秒杀服务
│   └── consumer/        # Kafka 订单消费者
├── internal/
│   ├── grpc/            # gRPC 服务实现
│   ├── handler/         # HTTP 处理器
│   ├── middleware/      # 中间件 (限流等)
│   ├── model/           # 数据模型
│   └── service/         # 业务逻辑层
├── pkg/
│   ├── kafka/           # Kafka 封装
│   └── redis/           # Redis 封装
├── scripts/             # SQL 脚本
├── docker-compose.yml   # 基础设施
└── Makefile
```

## 快速开始

### 1. 启动基础设施

```bash
make docker-up
```

### 2. 生成 gRPC 代码

```bash
make proto
```

### 3. 预热库存到 Redis

```bash
make preload-stock
```

### 4. 启动服务 (分别在三个终端)

```bash
# 终端 1: 秒杀 gRPC 服务
make run-seckill

# 终端 2: 订单消费者
make run-consumer

# 终端 3: API 网关
make run-gateway
```

### 5. 测试秒杀接口

```bash
curl -X POST http://localhost:8080/api/seckill/do \
  -H "Content-Type: application/json" \
  -d '{"user_id": 1, "goods_id": 1}'
```

## 核心设计

### 防超卖 - Redis Lua 脚本

```lua
-- 检查重复秒杀 -> 检查库存 -> 原子扣减 -> 记录用户
```

### 流量漏斗

1. **网关层**: 令牌桶限流 + IP 限流
2. **缓存层**: Redis 拦截 99% 请求
3. **消息队列**: 异步削峰
4. **数据库**: 乐观锁兜底
