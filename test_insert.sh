#!/bin/bash

# 测试行情数据插入脚本

echo "测试行情数据插入功能..."

# 测试单个 JSON 对象
echo "1. 测试单个 JSON 对象插入..."
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

echo -e "\n\n2. 测试 JSON 数组批量插入..."
curl -X POST http://127.0.0.1:8080/ingest \
  -H 'Content-Type: application/json' \
  -d '[
    {
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
    },
    {
      "receive_time": 1724131200.223456,
      "symbol": "ethusdt",
      "exchange": "binance",
      "market_type": "spot",
      "best_bid_px": 3000,
      "best_bid_sz": 1.0,
      "best_ask_px": 3001,
      "best_ask_sz": 0.8,
      "bids_px": [3000, 2999],
      "bids_sz": [2, 3],
      "asks_px": [3001, 3002],
      "asks_sz": [2.5, 3.5]
    }
  ]'

echo -e "\n\n3. 测试 NDJSON 格式..."
curl -X POST http://127.0.0.1:8080/ingest \
  -H 'Content-Type: application/json' \
  -d '{"receive_time": 1724131200.323456, "symbol": "solusdt", "exchange": "binance", "market_type": "spot", "best_bid_px": 100, "best_bid_sz": 10, "best_ask_px": 101, "best_ask_sz": 8, "bids_px": [100, 99], "bids_sz": [5, 6], "asks_px": [101, 102], "asks_sz": [7, 8]}
{"receive_time": 1724131200.423456, "symbol": "adausdt", "exchange": "binance", "market_type": "spot", "best_bid_px": 0.5, "best_bid_sz": 1000, "best_ask_px": 0.51, "best_ask_sz": 800, "bids_px": [0.5, 0.49], "bids_sz": [500, 600], "asks_px": [0.51, 0.52], "asks_sz": [400, 500]}'

echo -e "\n\n4. 检查健康状态..."
curl http://127.0.0.1:8080/healthz

echo -e "\n\n测试完成！" 