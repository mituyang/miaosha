FROM golang:1.24-alpine AS builder

WORKDIR /app

# 安装依赖
COPY go.mod go.sum ./
RUN go mod download

# 编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/worker ./cmd/worker

# 运行镜像
FROM alpine:3.19

# 安装时区数据
RUN apk add --no-cache tzdata

WORKDIR /app

COPY --from=builder /app/bin/api /app/api
COPY --from=builder /app/bin/worker /app/worker
COPY --from=builder /app/configs /app/configs

EXPOSE 8080

CMD ["/app/api"]
