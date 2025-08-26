#!/bin/bash

# 等待ClickHouse就绪的脚本

echo "Waiting for ClickHouse to be ready..."

# 等待ClickHouse启动
until clickhouse-client --host localhost --port 9000 --query "SELECT 1" > /dev/null 2>&1; do
    echo "ClickHouse is not ready yet, waiting..."
    sleep 2
done

echo "ClickHouse is ready!"

# 等待额外的几秒确保完全就绪
sleep 5

echo "ClickHouse initialization completed!" 