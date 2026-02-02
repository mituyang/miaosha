# 高并发秒杀系统

一个基于 Go + Vue 3 的高并发秒杀系统，采用 Redis + Kafka + MySQL 架构，实现了库存分段、异步削峰、超时自动取消等核心功能。

## 性能指标

在 **Mac Mini M4** 本地环境（Docker 部署）下的压测结果：

- **秒杀QPS**: 14,882 req/s (API 响应速度)
- **订单落库 TPS**: 12,124 order/s (订单落库速度)
- **平均延迟**: 33.55 ms
- **P99延迟**: 100.75 ms
- **成功率**: 100%
- **测试时长**: 30 秒
- **总请求数**: 446,964 次

## 技术栈

### 后端
- **Go 1.24** - 高性能 Web 服务
- **Gin** - HTTP 框架
- **GORM** - ORM 框架
- **MySQL 8.0** - 持久化存储
- **Redis 7** - 缓存 + 分布式锁 + 延迟队列
- **Kafka** - 消息队列（异步削峰）
- **JWT** - 用户认证

### 前端
- **Vue 3** - 渐进式框架
- **Vue Router** - 路由管理
- **Axios** - HTTP 客户端
- **Vite** - 构建工具

### 基础设施
- **Docker Compose** - 容器编排
- **Nginx** - 前端静态资源服务

## 核心特性

### 1. 库存分段 (Stock Segmentation)
- 将库存分散到 32 个 Redis 分段，减少锁竞争
- 使用 Lua 脚本保证原子性操作
- 支持动态调整分段数量

### 2. 异步削峰 (Async Peak Clipping)
- Redis 快速校验 + 扣减库存
- Kafka 异步落库（64 分区并行消费）
- 批量写入 MySQL（1000 条/批）

### 3. 限购控制 (Purchase Limit)
- Redis Bitmap 记录用户购买记录
- 支持配置每用户每商品最大购买数量
- Lua 脚本原子性检查 + 扣减

### 4. 订单超时自动取消
- **Redis 延迟队列**: ZSET 实现，500ms 扫描间隔
- **MySQL 兜底扫描**: 5 分钟扫描间隔
- 超时订单自动取消 + 库存返还

### 5. 分布式 ID
- 雪花算法生成订单 ID
- 支持 1024 个工作节点
- 毫秒级时间戳 + 序列号

### 6. 高性能优化
- **Kafka 批量发送**: 1000 条/批，5ms 超时
- **Kafka 批量消费**: 2000 条/次，50ms 超时
- **MySQL 批量写入**: 1000 条/批，50ms 刷新
- **连接池优化**: MySQL 2000 连接，Redis 2000 连接

## 系统架构

```
┌─────────┐      ┌─────────┐      ┌─────────┐
│ 前端    │─────▶│ Nginx   │─────▶│ Backend │
│ Vue 3   │      │         │      │ (Gin)   │
└─────────┘      └─────────┘      └────┬────┘
                                        │
                    ┌───────────────────┼───────────────────┐
                    ▼                   ▼                   ▼
              ┌─────────┐         ┌─────────┐        ┌─────────┐
              │ Redis   │         │ Kafka   │        │ MySQL   │
              │ 库存    │         │ 消息队列│        │ 持久化  │
              │ 分布式锁│         │ 64分区  │        │         │
              │ 延迟队列│         └────┬────┘        └─────────┘
              └─────────┘              │
                                       ▼
                                 ┌─────────┐
                                 │ Worker  │
                                 │ 消费者  │
                                 │ 批量写入│
                                 └─────────┘
```

## 快速开始

### 前置要求
- Docker & Docker Compose
- Go 1.24+ (本地开发)
- Node.js 18+ (本地开发)

### 一键启动

```bash
# 克隆项目
git clone <repository-url>
cd seckill

# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f
```

服务地址：
- 前端: http://localhost
- 后端 API: http://localhost:8080
- MySQL: localhost:13306
- Redis: localhost:16379
- Kafka: localhost:29092

### 本地开发

#### 后端开发
```bash
cd backend

# 安装依赖
go mod download

# 启动 API 服务
go run cmd/api/main.go

# 启动 Worker 服务
go run cmd/worker/main.go
```

#### 前端开发
```bash
cd frontend

# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 构建生产版本
npm run build
```

## 使用指南

### 1. 注册用户
```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"user1","password":"123456"}'
```

### 2. 登录获取 Token
```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"user1","password":"123456"}'
```

### 3. 预热库存
```bash
curl -X POST http://localhost:8080/api/admin/warmup \
  -H "X-Admin-Secret: tdPrNHfDnVCq+cQv8YvyW01dni0KVQ8maB0QracsWN8="
```

### 4. 参与秒杀
```bash
curl -X POST http://localhost:8080/api/seckill/buy \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <your-token>" \
  -d '{"goods_id":1,"quantity":1}'
```

### 5. 查询订单
```bash
curl http://localhost:8080/api/order/list \
  -H "Authorization: Bearer <your-token>"
```

## 压力测试

### 准备测试数据

```bash
cd test/tools

# 生成 10 万个测试用户
go run setup_users.go

# 生成 10 万个 Token
go run gen_tokens.go
```

### 执行压测

```bash
cd test

# 修改 benchmark.go 中的配置
# - concurrency: 并发数
# - targetQPS: 目标 QPS
# - maxUsers: 最大用户数
# - duration: 测试时长

# 运行压测
go run benchmark.go
```

### 最新压测报告 (2026-01-29)

**测试环境**: Mac Mini M4 + Docker

**核心指标**:
- 秒杀QPS: 14,882.21 req/s
- 系统QPS: 14,882.09 req/s
- 订单落库 TPS: 12,124.35 order/s
- 平均延迟: 33.55 ms
- P50延迟: 28.80 ms
- P95延迟: 70.83 ms
- P99延迟: 100.75 ms

**请求统计**:
- 实际耗时: 30.00 秒
- 总请求数: 446,964
- 完成请求: 446,464
- 成功请求: 446,464
- 成功率: 100%
- 失败请求: 0
- 被取消请求: 500

**延迟分布**:
- 0-10ms: 14,239 次
- 10-20ms: 98,931 次
- 20-50ms: 261,447 次
- 50-100ms: 67,221 次
- 100-200ms: 4,236 次
- 200-500ms: 390 次

**订单落库 性能**:
- MySQL订单数: 446,964
- 订单落库 TPS: 12,124.35 order/s (从请求到落库)

详细报告请查看: `test/benchmark_report_20260129_102831.html`

## 配置说明

主要配置文件: `backend/configs/config.yaml`

### Redis 配置
```yaml
redis:
  addr: 127.0.0.1:6379
  pool_size: 2000
  segment_count: 32  # 库存分段数量
```

### Kafka 配置
```yaml
kafka:
  brokers:
    - 127.0.0.1:29092
  producer:
    batch_size: 1000
    batch_timeout_ms: 5
    sender_count: 100
  consumer:
    consumer_count: 64
    batch_size: 1000
    batch_flush_ms: 50
```

### 超时配置
```yaml
timeout:
  order_timeout_seconds: 60
  redis_scan_interval: "500ms"
  mysql_scan_interval: "5m"
```

## 数据库设计

### 订单表 (orders)
```sql
CREATE TABLE orders (
    id BIGINT UNSIGNED NOT NULL COMMENT '订单ID(雪花算法)',
    user_id BIGINT UNSIGNED NOT NULL,
    goods_id BIGINT UNSIGNED NOT NULL,
    quantity INT UNSIGNED NOT NULL,
    pay_amount DECIMAL(10, 2) NOT NULL,
    status TINYINT UNSIGNED NOT NULL DEFAULT 0,
    request_time DATETIME(3) NOT NULL,
    create_time DATETIME(3) NOT NULL,
    born_time DATETIME(3) NOT NULL,
    store_time DATETIME(3) NOT NULL,
    write_time DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_user_id (user_id),
    INDEX idx_status_write_time (status, write_time)
) ENGINE=InnoDB;
```

### 商品表 (goods)
```sql
CREATE TABLE goods (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    product_name VARCHAR(255) NOT NULL,
    stock INT UNSIGNED NOT NULL DEFAULT 0,
    price DECIMAL(10, 2) NOT NULL,
    version INT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (id)
) ENGINE=InnoDB;
```

## API 文档

### 认证接口

#### 注册
- **POST** `/api/auth/register`
- Body: `{"username":"string","password":"string"}`

#### 登录
- **POST** `/api/auth/login`
- Body: `{"username":"string","password":"string"}`
- Response: `{"code":0,"data":{"token":"string"}}`

### 秒杀接口

#### 参与秒杀
- **POST** `/api/seckill/buy`
- Headers: `Authorization: Bearer <token>`
- Body: `{"goods_id":1,"quantity":1}`

#### 查询库存
- **GET** `/api/seckill/stock/:goods_id`

#### 预热库存
- **POST** `/api/admin/warmup`
- Headers: `X-Admin-Secret: <secret>`

### 订单接口

#### 查询订单列表
- **GET** `/api/order/list`
- Headers: `Authorization: Bearer <token>`

#### 支付订单
- **POST** `/api/order/pay/:order_id`
- Headers: `Authorization: Bearer <token>`

## 项目结构

```
.
├── backend/                 # 后端服务
│   ├── cmd/                # 入口文件
│   │   ├── api/           # API 服务
│   │   └── worker/        # Worker 服务
│   ├── internal/          # 内部代码
│   │   ├── config/        # 配置
│   │   ├── dto/           # 数据传输对象
│   │   ├── handler/       # HTTP 处理器
│   │   ├── middleware/    # 中间件
│   │   ├── model/         # 数据模型
│   │   ├── repository/    # 数据访问层
│   │   ├── service/       # 业务逻辑层
│   │   └── worker/        # 后台任务
│   ├── pkg/               # 公共包
│   │   ├── database/      # 数据库
│   │   ├── jwt/           # JWT
│   │   ├── logger/        # 日志
│   │   ├── mq/            # 消息队列
│   │   ├── redis/         # Redis
│   │   └── util/          # 工具函数
│   └── scripts/           # SQL 脚本
├── frontend/              # 前端服务
│   ├── src/
│   │   ├── components/    # 组件
│   │   ├── views/         # 页面
│   │   ├── router/        # 路由
│   │   └── api.js         # API 封装
│   └── vite.config.js
├── test/                  # 压测工具
│   ├── benchmark.go       # 压测脚本
│   └── tools/             # 辅助工具
└── docker-compose.yml     # Docker 编排
```

## 技术亮点

1. **库存分段**: 将库存分散到多个 Redis 分段，减少锁竞争，提升并发能力
2. **Lua 脚本**: 保证 Redis 操作的原子性，避免超卖
3. **异步削峰**: Kafka 消息队列异步处理订单，保护数据库
4. **批量优化**: Kafka 批量发送/消费，MySQL 批量写入，大幅提升吞吐量
5. **延迟队列**: Redis ZSET 实现订单超时自动取消
6. **分布式锁**: 防止库存预热并发冲突
7. **雪花算法**: 分布式 ID 生成，支持水平扩展
8. **连接池优化**: MySQL/Redis 连接池调优，支持高并发

## 监控与日志

日志文件位于 `logs/` 目录：
- `backend.log` - API 服务日志
- `worker.log` - Worker 服务日志
- `mysql.log` - MySQL 日志
- `redis.log` - Redis 日志
- `kafka.log` - Kafka 日志

## 常见问题

### 1. 启动失败？
检查端口占用：13306 (MySQL), 16379 (Redis), 8080 (Backend), 80 (Frontend)

### 2. 库存不一致？
执行库存预热：`POST /api/admin/warmup`

### 3. 订单未支付自动取消？
默认 60 秒超时，可在 `config.yaml` 中修改 `timeout.order_timeout_seconds`

### 4. 压测 QPS 上不去？
- 检查 Kafka 分区数（默认 64）
- 调整 Worker 消费协程数（默认 64）
- 增加 MySQL/Redis 连接池大小

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！
