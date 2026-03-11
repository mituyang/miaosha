#!/bin/bash
set -euo pipefail

# Kafka 启动前清理脚本
# 解决 Zookeeper 中残留的 broker 临时节点问题

ZOOKEEPER_CONNECT="${KAFKA_ZOOKEEPER_CONNECT:-zookeeper:2181}"
BROKER_ID="${KAFKA_BROKER_ID:-1}"
MAX_RETRIES=30
RETRY_INTERVAL=2

echo "Kafka startup config: ZOOKEEPER_CONNECT=${ZOOKEEPER_CONNECT}, BROKER_ID=${BROKER_ID}"
echo "Kafka JVM options: KAFKA_HEAP_OPTS=${KAFKA_HEAP_OPTS:-<unset>}"
echo "Waiting for Zookeeper to be ready..."
for i in $(seq 1 $MAX_RETRIES); do
    zk_output="$(zookeeper-shell "$ZOOKEEPER_CONNECT" ls / 2>&1 || true)"
    if printf '%s\n' "$zk_output" | grep -q "Error occurred during initialization of VM"; then
        echo "Zookeeper probe failed because the JVM could not start:"
        printf '%s\n' "$zk_output"
        exit 1
    fi
    if printf '%s\n' "$zk_output" | grep -q "Exception"; then
        echo "Zookeeper probe returned an exception on attempt $i/$MAX_RETRIES:"
        printf '%s\n' "$zk_output"
    fi
    if printf '%s\n' "$zk_output" | grep -q "\\[.*brokers.*\\]"; then
        echo "Zookeeper is ready"
        break
    fi
    if [ $i -eq $MAX_RETRIES ]; then
        echo "Zookeeper is not ready after $MAX_RETRIES attempts"
        echo "Last zookeeper-shell output:"
        printf '%s\n' "$zk_output"
        exit 1
    fi
    echo "Waiting for Zookeeper... ($i/$MAX_RETRIES)"
    sleep $RETRY_INTERVAL
done

# 清理旧的 broker 注册节点
echo "Cleaning up stale broker registration for broker $BROKER_ID..."
cleanup_output="$(echo "deleteall /brokers/ids/$BROKER_ID" | zookeeper-shell "$ZOOKEEPER_CONNECT" 2>&1 || true)"
printf '%s\n' "$cleanup_output"
echo "Cleanup completed"

# 启动 Kafka
echo "Starting Kafka broker $BROKER_ID..."
exec /etc/confluent/docker/run
