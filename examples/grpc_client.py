#!/usr/bin/env python3
"""
gRPC 客户端示例 - 行情数据推送

使用方法:
1. 确保已安装依赖: pip install grpcio grpcio-tools
2. 生成 protobuf 代码: python -m grpc_tools.protoc -I../proto --python_out=. --grpc_python_out=. ../proto/quota_service.proto
3. 运行客户端: python grpc_client.py
"""

import grpc
import time
import random
import sys
import os

# 添加proto目录到路径
sys.path.append(os.path.join(os.path.dirname(__file__), 'proto'))

try:
    import quota_service_pb2
    import quota_service_pb2_grpc
except ImportError:
    print("请先生成 protobuf 代码:")
    print("python -m grpc_tools.protoc -I../proto --python_out=. --grpc_python_out=. ../proto/quota_service.proto")
    sys.exit(1)


def create_sample_tick():
    """创建示例行情数据"""
    return quota_service_pb2.Tick(
        receive_time=time.time(),
        symbol="btcusdt",
        exchange="binance",
        market_type="spot",
        best_bid_px=60000.0,
        best_bid_sz=0.5,
        best_ask_px=60001.0,
        best_ask_sz=0.4,
        bids_px=[60000, 59999, 59998],
        bids_sz=[1, 2, 3],
        asks_px=[60001, 60002, 60003],
        asks_sz=[1.5, 2.5, 3.5]
    )


def create_sample_ticks(count):
    """创建多个示例行情数据"""
    ticks = []
    symbols = ["btcusdt", "ethusdt", "bnbusdt", "adausdt", "dogeusdt"]
    exchanges = ["binance", "okx", "bybit", "gate", "kucoin"]
    
    for i in range(count):
        tick = quota_service_pb2.Tick(
            receive_time=time.time() + i,
            symbol=symbols[i % len(symbols)],
            exchange=exchanges[i % len(exchanges)],
            market_type="spot",
            best_bid_px=50000 + i * 100,
            best_bid_sz=0.1 + i * 0.1,
            best_ask_px=50001 + i * 100,
            best_ask_sz=0.1 + i * 0.1,
            bids_px=[50000 + i * 100, 49999 + i * 100],
            bids_sz=[1, 2],
            asks_px=[50001 + i * 100, 50002 + i * 100],
            asks_sz=[1.5, 2.5]
        )
        ticks.append(tick)
    
    return ticks


def create_random_tick():
    """创建随机行情数据"""
    base_price = 50000 + random.random() * 20000  # 50000-70000
    spread = 1 + random.random() * 10  # 1-11
    
    return quota_service_pb2.Tick(
        receive_time=time.time(),
        symbol="btcusdt",
        exchange="binance",
        market_type="spot",
        best_bid_px=base_price,
        best_bid_sz=0.1 + random.random() * 2,
        best_ask_px=base_price + spread,
        best_ask_sz=0.1 + random.random() * 2,
        bids_px=[base_price, base_price - 1, base_price - 2],
        bids_sz=[1 + random.random() * 5, 2 + random.random() * 5, 3 + random.random() * 5],
        asks_px=[base_price + spread, base_price + spread + 1, base_price + spread + 2],
        asks_sz=[1 + random.random() * 5, 2 + random.random() * 5, 3 + random.random() * 5]
    )


def main():
    """主函数"""
    # 连接到gRPC服务器
    with grpc.insecure_channel('localhost:9090') as channel:
        stub = quota_service_pb2_grpc.QuotaServiceStub(channel)
        
        try:
            # 测试健康检查
            print("Testing health check...")
            health_request = quota_service_pb2.HealthCheckRequest()
            health_response = stub.HealthCheck(health_request)
            print(f"Health check response: {health_response}")
            
            # 测试单个行情推送
            print("\nTesting single tick ingestion...")
            tick = create_sample_tick()
            request = quota_service_pb2.IngestTickRequest(tick=tick)
            response = stub.IngestTick(request)
            print(f"Single tick response: {response}")
            
            # 测试批量行情推送
            print("\nTesting batch tick ingestion...")
            ticks = create_sample_ticks(10)
            batch_request = quota_service_pb2.IngestTicksRequest(ticks=ticks)
            batch_response = stub.IngestTicks(batch_request)
            print(f"Batch ticks response: {batch_response}")
            
            # 持续推送数据（模拟实时行情）
            print("\nStarting continuous data ingestion...")
            print("Press Ctrl+C to stop...")
            
            count = 0
            start_time = time.time()
            
            while True:
                try:
                    tick = create_random_tick()
                    request = quota_service_pb2.IngestTickRequest(tick=tick)
                    response = stub.IngestTick(request)
                    
                    count += 1
                    if count % 100 == 0:
                        elapsed = time.time() - start_time
                        rate = count / elapsed
                        print(f"Ingested {count} ticks, rate: {rate:.2f} ticks/sec")
                    
                    time.sleep(0.1)  # 每100ms推送一次
                    
                except KeyboardInterrupt:
                    print(f"\nStopped. Total ingested: {count} ticks")
                    break
                except Exception as e:
                    print(f"Error ingesting tick: {e}")
                    continue
                    
        except grpc.RpcError as e:
            print(f"gRPC error: {e.code()}: {e.details()}")
        except Exception as e:
            print(f"Unexpected error: {e}")


if __name__ == '__main__':
    main() 