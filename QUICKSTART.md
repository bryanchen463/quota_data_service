# 🚀 快速开始指南

## 5分钟快速部署

### 1. 启动 ClickHouse

```bash
# 使用 Docker Compose 启动 ClickHouse
docker-compose up -d clickhouse

# 等待服务启动（约30秒）
sleep 30
```

### 2. 启动行情数据服务

```bash
# 编译项目
go build -o quota_data_service .

# 启动服务
./quota_data_service
```

### 3. 测试数据插入

```bash
# 给测试脚本添加执行权限
chmod +x test_insert.sh

# 运行测试
./test_insert.sh
```

## 🧪 快速测试

### 测试单个数据插入

```bash
curl -X POST http://127.0.0.1:8080/ingest \
  -H 'Content-Type: application/json' \
  -d '{
    "receive_time": 1724131200.123456,
    "symbol": "btcusdt",
    "exchange": "binance",
    "market_type": "spot",
    "best_bid_px": 60000,
    "best_bid_sz": 0.5,
    "best_ask_px": 60001,
    "best_ask_sz": 0.4,
    "bids_px": [60000, 59999],
    "bids_sz": [1, 2],
    "asks_px": [60001, 60002],
    "asks_sz": [1.5, 2.5]
  }'
```

### 检查服务状态

```bash
# 健康检查
curl http://127.0.0.1:8080/healthz

# 查看日志
docker logs quota-data-service
```

## 📊 验证数据

### 连接到 ClickHouse

```bash
# 进入 ClickHouse 容器
docker exec -it clickhouse clickhouse-client

# 查看数据
SELECT count() FROM crypto_market.raw_ticks;
SELECT * FROM crypto_market.raw_ticks ORDER BY receive_time DESC LIMIT 5;
```

## ⚠️ 常见问题

### 端口被占用
```bash
# 检查端口占用
netstat -tlnp | grep -E ':(8080|9090)'

# 停止现有服务
sudo pkill quota_data_service
```

### ClickHouse 连接失败
```bash
# 检查 ClickHouse 状态
docker ps | grep clickhouse

# 重启 ClickHouse
docker-compose restart clickhouse
```

## 🔧 配置调优

### 生产环境配置

```bash
# 高并发场景
export FLUSH_MS=5
export MAX_BATCH=1000
export MAX_QUEUE=50000
export INSERT_TIMEOUT_MS=60000

# 启动服务
./quota_data_service
```

### 低延迟场景

```bash
# 低延迟配置
export FLUSH_MS=1
export MAX_BATCH=100
export MAX_QUEUE=10000

# 启动服务
./quota_data_service
```

## 📈 性能监控

### 查看实时指标

```bash
# 监控服务日志
docker logs -f quota-data-service

# 查看 ClickHouse 性能
docker exec -it clickhouse clickhouse-client --query "
SELECT 
    metric,
    value
FROM system.metrics 
WHERE metric LIKE '%Insert%' OR metric LIKE '%Query%'
ORDER BY metric;
"
```

---

**下一步**：查看完整 [README.md](README.md) 了解更多详细信息！ 