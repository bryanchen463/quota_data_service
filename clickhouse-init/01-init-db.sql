-- ClickHouse 数据库初始化脚本

-- 创建数据库（如果不存在）
CREATE DATABASE IF NOT EXISTS crypto_market;

-- 使用数据库
USE crypto_market;

-- 创建行情数据表（如果不存在）
CREATE TABLE IF NOT EXISTS raw_ticks (
    symbol         LowCardinality(String),
    exchange       LowCardinality(String),
    market_type    LowCardinality(String),
    receive_time   DateTime64(6) CODEC(DoubleDelta, ZSTD(3)),
    best_bid_px    Float64,
    best_bid_sz    Float64,
    best_ask_px    Float64,
    best_ask_sz    Float64,
    bids_px        Array(Float64),
    bids_sz        Array(Float64),
    asks_px        Array(Float64),
    asks_sz        Array(Float64)
) ENGINE = ReplacingMergeTree
PARTITION BY toDate(receive_time)
ORDER BY (symbol, receive_time, exchange, market_type)
SETTINGS index_granularity = 8192;

-- 创建默认用户权限
CREATE USER IF NOT EXISTS default IDENTIFIED WITH no_password;
GRANT ALL ON crypto_market.* TO default;

-- 显示创建的表
SHOW TABLES FROM crypto_market; 