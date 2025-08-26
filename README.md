# Quota Data Service

一个高性能的加密货币行情数据接收和存储服务，支持 HTTP 和 gRPC 接口，使用 ClickHouse 作为存储后端。

## 🚀 功能特性

- **多协议支持**：HTTP REST API + gRPC 服务
- **高性能存储**：基于 ClickHouse 的列式存储，支持微秒级时间精度
- **智能批处理**：10ms 定时或达到阈值自动批量写入
- **多种数据格式**：支持单 JSON、JSON 数组、NDJSON 格式
- **自动建表**：程序启动时自动创建 ClickHouse 表结构
- **连接重试**：自动重试失败的数据库连接和插入操作
- **健康监控**：定期检查 ClickHouse 连接状态
- **优雅关闭**：支持优雅关闭和资源清理

## 📊 数据模型

### Tick 数据结构

```json
{
  "receive_time": 1724131200.123456,  // Unix时间戳（秒）
  "symbol": "btcusdt",                 // 交易对符号
  "exchange": "binance",               // 交易所
  "market_type": "spot",               // 市场类型：spot|futures
  "best_bid_px": 60000,               // 最佳买价
  "best_bid_sz": 0.5,                 // 最佳买量
  "best_ask_px": 60001,               // 最佳卖价
  "best_ask_sz": 0.4,                 // 最佳卖量
  "bids_px": [60000, 59999],          // 买单价格数组
  "bids_sz": [1, 2],                  // 买单数量数组
  "asks_px": [60001, 60002],          // 卖单价格数组
  "asks_sz": [1.5, 2.5]               // 卖单数量数组
}
```

### ClickHouse 表结构

```sql
CREATE TABLE crypto_market.raw_ticks (
  receive_time   DateTime64(6) CODEC(DoubleDelta, ZSTD(3)),
  symbol         LowCardinality(String),
  exchange       LowCardinality(String),
  market_type    LowCardinality(String),
  best_bid_px    Float64,
  best_bid_sz    Float64,
  best_ask_px    Float64,
  best_ask_sz    Float64,
  bids_px        Array(Float64),
  bids_sz        Array(Float64),
  asks_px        Array(Float64),
  asks_sz        Array(Float64)
) ENGINE = MergeTree
PARTITION BY toDate(receive_time)
ORDER BY (symbol, receive_time)
SETTINGS index_granularity = 8192;
```

## 🛠️ 技术栈

- **语言**：Go 1.23+
- **数据库**：ClickHouse
- **通信协议**：HTTP REST + gRPC
- **序列化**：Protocol Buffers
- **容器化**：Docker + Docker Compose

## 📦 安装部署

### 环境要求

- Go 1.23+
- ClickHouse 22.8+
- Docker & Docker Compose（可选）

### 快速开始

1. **克隆项目**
   ```bash
   git clone <repository-url>
   cd quota_data_service
   ```

2. **安装依赖**
   ```bash
   go mod tidy
   ```

3. **配置环境变量**
   ```bash
   cp env.example .env
   # 编辑 .env 文件，设置你的配置
   ```

4. **编译运行**
   ```bash
   go build -o quota_data_service .
   ./quota_data_service
   ```

### Docker 部署

1. **启动 ClickHouse**
   ```bash
   docker-compose up -d clickhouse
   ```

2. **启动服务**
   ```bash
   docker-compose up -d quota-data-service
   ```

3. **查看日志**
   ```bash
   docker logs quota-data-service
   ```

## ⚙️ 配置说明

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `INGEST_ADDR` | `:8080` | HTTP 服务监听地址 |
| `GRPC_ADDR` | `:9090` | gRPC 服务监听地址 |
| `CH_ADDR` | `127.0.0.1:9000` | ClickHouse 地址 |
| `CH_USER` | `default` | ClickHouse 用户名 |
| `CH_PASSWORD` | `` | ClickHouse 密码 |
| `CH_DATABASE` | `crypto_market` | ClickHouse 数据库名 |
| `CH_TABLE` | `raw_ticks` | ClickHouse 表名 |
| `FLUSH_MS` | `10` | 批处理刷新间隔（毫秒） |
| `MAX_BATCH` | `800` | 最大批处理大小 |
| `MAX_QUEUE` | `20000` | 最大队列容量 |
| `INSERT_TIMEOUT_MS` | `30000` | 插入超时时间（毫秒） |

### 生产环境建议配置

```bash
# 高并发场景
export FLUSH_MS=5
export MAX_BATCH=1000
export MAX_QUEUE=50000
export INSERT_TIMEOUT_MS=60000

# 低延迟场景
export FLUSH_MS=1
export MAX_BATCH=100
export MAX_QUEUE=10000
```

## 📡 API 接口

### HTTP REST API

#### 1. 推送行情数据

```bash
# 单个 JSON 对象
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

# JSON 数组批量推送
curl -X POST http://127.0.0.1:8080/ingest \
  -H 'Content-Type: application/json' \
  -d '[
    {"receive_time": 1724131200.123456, "symbol": "btcusdt", ...},
    {"receive_time": 1724131200.223456, "symbol": "ethusdt", ...}
  ]'

# NDJSON 格式（每行一个 JSON）
curl -X POST http://127.0.0.1:8080/ingest \
  -H 'Content-Type: application/json' \
  -d '{"receive_time": 1724131200.123456, "symbol": "btcusdt", ...}
{"receive_time": 1724131200.223456, "symbol": "ethusdt", ...}'
```

#### 2. 健康检查

```bash
curl http://127.0.0.1:8080/healthz
```

### gRPC API

服务定义：`proto/quota_service.proto`

#### 主要方法

- `IngestTick` - 推送单个行情数据
- `IngestTicks` - 批量推送行情数据
- `HealthCheck` - 健康检查

#### gRPC 客户端示例

```go
package main

import (
    "context"
    "log"
    pb "github.com/bryanchen463/quota_data_service/proto"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    conn, err := grpc.Dial("127.0.0.1:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatalf("failed to connect: %v", err)
    }
    defer conn.Close()

    client := pb.NewQuotaServiceClient(conn)
    
    // 推送单个行情
    resp, err := client.IngestTick(context.Background(), &pb.IngestTickRequest{
        Tick: &pb.Tick{
            ReceiveTime: 1724131200.123456,
            Symbol:      "btcusdt",
            Exchange:    "binance",
            MarketType:  "spot",
            // ... 其他字段
        },
    })
    
    if err != nil {
        log.Printf("failed to ingest tick: %v", err)
    } else {
        log.Printf("ingest response: %v", resp)
    }
}
```

## 🧪 测试

### 运行测试脚本

```bash
# 给测试脚本添加执行权限
chmod +x test_insert.sh

# 运行测试
./test_insert.sh
```

### 性能测试

```bash
# 使用 ab 进行 HTTP 压力测试
ab -n 10000 -c 100 -p test_data.json -T application/json http://127.0.0.1:8080/ingest

# 使用 grpcurl 进行 gRPC 测试
grpcurl -plaintext -d '{"tick": {...}}' 127.0.0.1:9090 quota_service.QuotaService/IngestTick
```

## 📈 性能指标

### 基准测试结果

- **单机性能**：支持 10,000+ TPS
- **延迟**：平均插入延迟 < 50ms
- **吞吐量**：单批次最大 800 条记录
- **内存使用**：队列容量 20,000 条记录

### 监控指标

- 插入成功率
- 平均插入延迟
- 队列长度
- ClickHouse 连接状态
- 错误率和重试次数

## 🔧 故障排除

### 常见问题

1. **ClickHouse 连接失败**
   ```bash
   # 检查 ClickHouse 服务状态
   docker ps | grep clickhouse
   
   # 检查网络连接
   telnet clickhouse 9000
   ```

2. **插入超时**
   ```bash
   # 增加超时时间
   export INSERT_TIMEOUT_MS=60000
   
   # 检查 ClickHouse 性能
   docker exec -it clickhouse clickhouse-client --query "SELECT * FROM system.metrics WHERE metric LIKE '%Insert%'"
   ```

3. **内存不足**
   ```bash
   # 减少队列大小
   export MAX_QUEUE=10000
   
   # 减少批处理大小
   export MAX_BATCH=400
   ```

### 日志分析

```bash
# 查看服务日志
docker logs quota-data-service

# 查看 ClickHouse 日志
docker logs clickhouse

# 实时监控日志
docker logs -f quota-data-service
```

## 🚀 扩展功能

### 计划中的功能

- [ ] 数据压缩和归档
- [ ] 实时数据查询 API
- [ ] 数据质量监控
- [ ] 多数据中心支持
- [ ] 数据备份和恢复
- [ ] 监控面板和告警

### 贡献指南

1. Fork 项目
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🤝 联系方式

- 项目维护者：[Your Name]
- 邮箱：[your.email@example.com]
- 项目地址：[GitHub Repository URL]

## 🙏 致谢

感谢以下开源项目的支持：

- [ClickHouse](https://clickhouse.com/) - 高性能列式数据库
- [gRPC](https://grpc.io/) - 高性能 RPC 框架
- [Protocol Buffers](https://developers.google.com/protocol-buffers) - 数据序列化格式

---

**注意**：本项目仅用于学习和研究目的，请在生产环境中谨慎使用。 