# 行情数据服务 - gRPC 支持

本服务现在同时支持 HTTP 和 gRPC 两种协议来接收行情数据。

## 功能特性

- **HTTP API**: 原有的 `/ingest` 端点，支持 JSON 和 NDJSON 格式
- **gRPC API**: 新增的 gRPC 服务，提供类型安全的接口
- **双协议支持**: 可以同时使用两种协议，数据统一处理
- **批量处理**: 支持单个和批量行情数据推送

## 快速开始

### 1. 安装依赖

```bash
# 安装 protobuf 编译器
make deps

# 或者手动安装
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 2. 生成代码

```bash
make proto
```

### 3. 构建和运行

```bash
make build
make run
```

## 配置

服务启动时会同时监听两个端口：

- **HTTP**: 默认 `:8080` (可通过 `INGEST_ADDR` 环境变量配置)
- **gRPC**: 默认 `:9090` (可通过 `GRPC_ADDR` 环境变量配置)

```bash
export INGEST_ADDR=":8080"
export GRPC_ADDR=":9090"
./quota_data_service
```

## gRPC 接口

### 服务定义

```protobuf
service QuotaService {
  // 推送单个行情数据
  rpc IngestTick(IngestTickRequest) returns (IngestTickResponse);
  
  // 批量推送行情数据
  rpc IngestTicks(IngestTicksRequest) returns (IngestTicksResponse);
  
  // 健康检查
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

### 数据模型

```protobuf
message Tick {
  double receive_time = 1;        // Unix时间戳（秒）
  string symbol = 2;              // 交易对符号
  string exchange = 3;            // 交易所
  string market_type = 4;         // 市场类型：spot|futures
  double best_bid_px = 5;         // 最佳买价
  double best_bid_sz = 6;         // 最佳买量
  double best_ask_px = 7;         // 最佳卖价
  double best_ask_sz = 8;         // 最佳卖量
  repeated double bids_px = 9;    // 买单价格数组
  repeated double bids_sz = 10;   // 买单数量数组
  repeated double asks_px = 11;   // 卖单价格数组
  repeated double asks_sz = 12;   // 卖单数量数组
}
```

## 客户端示例

### Go 客户端

```go
package main

import (
    "context"
    "log"
    "time"

    pb "github.com/bryanchen463/quota_data_service/proto"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    conn, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatalf("failed to connect: %v", err)
    }
    defer conn.Close()

    client := pb.NewQuotaServiceClient(conn)
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    // 推送单个行情
    tick := &pb.Tick{
        ReceiveTime: float64(time.Now().Unix()),
        Symbol:      "btcusdt",
        Exchange:    "binance",
        MarketType:  "spot",
        BestBidPx:   60000.0,
        BestBidSz:   0.5,
        BestAskPx:   60001.0,
        BestAskSz:   0.4,
        BidsPx:      []float64{60000, 59999},
        BidsSz:      []float64{1, 2},
        AsksPx:      []float64{60001, 60002},
        AsksSz:      []float64{1.5, 2.5},
    }

    resp, err := client.IngestTick(ctx, &pb.IngestTickRequest{Tick: tick})
    if err != nil {
        log.Fatalf("failed to ingest tick: %v", err)
    }
    log.Printf("Response: %v", resp)
}
```

### Python 客户端

```python
import grpc
import time
from proto import quota_service_pb2
from proto import quota_service_pb2_grpc

def main():
    with grpc.insecure_channel('localhost:9090') as channel:
        stub = quota_service_pb2_grpc.QuotaServiceStub(channel)
        
        # 创建行情数据
        tick = quota_service_pb2.Tick(
            receive_time=time.time(),
            symbol="btcusdt",
            exchange="binance",
            market_type="spot",
            best_bid_px=60000.0,
            best_bid_sz=0.5,
            best_ask_px=60001.0,
            best_ask_sz=0.4,
            bids_px=[60000, 59999],
            bids_sz=[1, 2],
            asks_px=[60001, 60002],
            asks_sz=[1.5, 2.5]
        )
        
        # 推送数据
        request = quota_service_pb2.IngestTickRequest(tick=tick)
        response = stub.IngestTick(request)
        print(f"Response: {response}")

if __name__ == '__main__':
    main()
```

### 使用 grpcurl 测试

```bash
# 安装 grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# 查看服务列表
grpcurl -plaintext localhost:9090 list

# 查看方法详情
grpcurl -plaintext localhost:9090 describe quota_service.QuotaService

# 调用健康检查
grpcurl -plaintext localhost:9090 quota_service.QuotaService/HealthCheck
```

## 性能特点

- **批量处理**: 支持批量推送，减少网络开销
- **异步写入**: 数据先进入内存队列，异步批量写入 ClickHouse
- **队列管理**: 自动处理队列满的情况，防止内存溢出
- **优雅关闭**: 支持优雅关闭，确保数据不丢失

## 监控和调试

- **健康检查**: HTTP `/healthz` 和 gRPC `HealthCheck`
- **反射支持**: gRPC 反射已启用，方便调试
- **日志记录**: 详细的日志输出，包括错误和性能信息

## 注意事项

1. **数据验证**: gRPC 接口会验证数据的完整性和有效性
2. **批量限制**: 单次批量推送限制为 1000 条记录
3. **队列容量**: 默认队列容量为 20000，可通过环境变量调整
4. **超时设置**: 插入超时默认为 1.5 秒，可通过环境变量调整

## 故障排除

### 常见问题

1. **protobuf 生成失败**: 确保已安装 `protoc` 和 Go 插件
2. **gRPC 连接失败**: 检查端口是否被占用，防火墙设置
3. **数据验证失败**: 检查必填字段和数据类型
4. **队列满**: 调整 `MAX_QUEUE` 环境变量或检查 ClickHouse 性能

### 日志分析

服务会输出详细的日志信息，包括：
- 启动配置
- 连接状态
- 数据处理统计
- 错误详情
- 性能指标 