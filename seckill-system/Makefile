.PHONY: proto build run-gateway run-seckill run-consumer docker-up docker-down test

# 生成 gRPC 代码
proto:
	protoc --go_out=. --go-grpc_out=. api/proto/seckill.proto

# 下载依赖
deps:
	go mod tidy

# 构建所有服务
build: deps
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/seckill ./cmd/seckill
	go build -o bin/consumer ./cmd/consumer

# 运行 API 网关
run-gateway:
	go run ./cmd/gateway

# 运行秒杀 gRPC 服务
run-seckill:
	go run ./cmd/seckill

# 运行订单消费者
run-consumer:
	go run ./cmd/consumer

# 启动基础设施 (MySQL, Redis, Kafka)
docker-up:
	docker-compose up -d

# 停止基础设施
docker-down:
	docker-compose down

# 预热库存到 Redis (示例命令)
preload-stock:
	redis-cli SET seckill:stock:1 100
	redis-cli SET seckill:stock:2 50

# 压力测试 (需要安装 wrk)
stress-test:
	wrk -t12 -c400 -d30s -s scripts/seckill.lua http://localhost:8080/api/seckill/do
