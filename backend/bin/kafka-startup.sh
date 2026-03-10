#!/bin/bash
set -e

# Kafka 启动前清理脚本
# 解决 Zookeeper 中残留的 broker 临时节点问题

ZOOKEEPER_CONNECT="${KAFKA_ZOOKEEPER_CONNECT:-zookeeper:2181}"
BROKER_ID="${KAFKA_BROKER_ID:-1}"
MAX_RETRIES=30
RETRY_INTERVAL=2

echo "Waiting for Zookeeper to be ready..."
for i in $(seq 1 $MAX_RETRIES); do
    if zookeeper-shell $ZOOKEEPER_CONNECT ls / 2>/dev/null | grep -q brokers; then
        echo "Zookeeper is ready"
        break
    fi
    if [ $i -eq $MAX_RETRIES ]; then
        echo "Zookeeper is not ready after $MAX_RETRIES attempts"
        exit 1
    fi
    echo "Waiting for Zookeeper... ($i/$MAX_RETRIES)"
    sleep $RETRY_INTERVAL
done

# 清理旧的 broker 注册节点
echo "Cleaning up stale broker registration for broker $BROKER_ID..."
echo "deleteall /brokers/ids/$BROKER_ID" | zookeeper-shell $ZOOKEEPER_CONNECT 2>/dev/null || true
echo "Cleanup completed"

# 启动 Kafka
echo "Starting Kafka broker $BROKER_ID..."
exec /etc/confluent/docker/run
