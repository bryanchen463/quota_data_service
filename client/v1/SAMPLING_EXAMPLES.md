# 时间间隔采样功能使用示例

## 概述

时间间隔采样功能允许您按指定的时间间隔获取行情数据，这对于数据分析和图表展示非常有用。系统支持从 1 毫秒到 1 天的各种时间间隔。

## 支持的时间间隔

| 间隔 | 枚举值 | 毫秒值 | 说明 |
|------|--------|--------|------|
| 1毫秒 | `INTERVAL_1MS` | 1 | 最高精度 |
| 5毫秒 | `INTERVAL_5MS` | 5 | 高频交易 |
| 10毫秒 | `INTERVAL_10MS` | 10 | 高频交易 |
| 30毫秒 | `INTERVAL_30MS` | 30 | 高频交易 |
| 100毫秒 | `INTERVAL_100MS` | 100 | 中频交易 |
| 1秒 | `INTERVAL_1S` | 1000 | 实时监控 |
| 5秒 | `INTERVAL_5S` | 5000 | 短期分析 |
| 10秒 | `INTERVAL_10S` | 10000 | 短期分析 |
| 30秒 | `INTERVAL_30S` | 30000 | 中期分析 |
| 1分钟 | `INTERVAL_1M` | 60000 | 技术分析 |
| 5分钟 | `INTERVAL_5M` | 300000 | 技术分析 |
| 15分钟 | `INTERVAL_15M` | 900000 | 技术分析 |
| 30分钟 | `INTERVAL_30M` | 1800000 | 技术分析 |
| 1小时 | `INTERVAL_1H` | 3600000 | 长期分析 |
| 4小时 | `INTERVAL_4H` | 14400000 | 长期分析 |
| 1天 | `INTERVAL_1D` | 86400000 | 日线分析 |

## 使用方法

### 1. 基本采样函数

```go
package main

import (
    "context"
    "time"
    "fmt"
    
    pb "github.com/bryanchen463/quota_data_service/proto"
    "github.com/bryanchen463/quota_data_service/client/v1"
)

func main() {
    ctx := context.Background()
    
    // 初始化连接池
    err := v1.InitPool(ctx, "localhost:9090", 1, 10, 30*time.Second, 5*time.Minute)
    if err != nil {
        panic(err)
    }
    
    // 获取过去1小时的1秒间隔数据
    ticks, err := v1.GetTicks1Second(ctx, "BTCUSDT", "binance", "spot", 
        time.Now().Add(-1*time.Hour).Unix(), time.Now().Unix())
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("获取到 %d 条数据\n", len(ticks))
    for _, tick := range ticks {
        fmt.Printf("时间: %v, 价格: %.2f/%.2f\n", 
            time.Unix(int64(tick.ReceiveTime), 0), 
            tick.BestBidPx, tick.BestAskPx)
    }
}
```

### 2. 自定义间隔采样

```go
// 使用自定义间隔和参数
ticks, err := v1.GetTicksWithInterval(ctx, "BTCUSDT", "binance", "spot",
    time.Now().Add(-24*time.Hour).Unix(), // 开始时间：24小时前
    time.Now().Unix(),                    // 结束时间：现在
    pb.Interval_INTERVAL_5M,              // 5分钟间隔
    100,                                  // 限制100条
    0)                                    // 偏移0条
```

### 3. 不同时间间隔的便捷函数

```go
// 1秒间隔 - 适合实时监控
ticks1s, err := v1.GetTicks1Second(ctx, "BTCUSDT", "binance", "spot", start, end)

// 5秒间隔 - 适合短期分析
ticks5s, err := v1.GetTicks5Second(ctx, "BTCUSDT", "binance", "spot", start, end)

// 1分钟间隔 - 适合技术分析
ticks1m, err := v1.GetTicks1Minute(ctx, "BTCUSDT", "binance", "spot", start, end)

// 5分钟间隔 - 适合中期分析
ticks5m, err := v1.GetTicks5Minute(ctx, "BTCUSDT", "binance", "spot", start, end)

// 1小时间隔 - 适合长期分析
ticks1h, err := v1.GetTicks1Hour(ctx, "BTCUSDT", "binance", "spot", start, end)

// 1天间隔 - 适合日线分析
ticks1d, err := v1.GetTicks1Day(ctx, "BTCUSDT", "binance", "spot", start, end)
```

## 实际应用场景

### 1. 实时价格监控

```go
// 每5秒获取一次最新价格
func monitorPrice(ctx context.Context) {
    for {
        ticks, err := v1.GetTicks5Second(ctx, "BTCUSDT", "binance", "spot",
            time.Now().Add(-10*time.Second).Unix(), time.Now().Unix())
        if err != nil {
            log.Printf("获取价格失败: %v", err)
            continue
        }
        
        if len(ticks) > 0 {
            latest := ticks[0]
            fmt.Printf("BTC价格: %.2f (买: %.2f, 卖: %.2f)\n", 
                (latest.BestBidPx + latest.BestAskPx) / 2,
                latest.BestBidPx, latest.BestAskPx)
        }
        
        time.Sleep(5 * time.Second)
    }
}
```

### 2. 技术指标计算

```go
// 获取5分钟K线数据计算移动平均线
func calculateMA(ctx context.Context, symbol string, periods int) ([]float64, error) {
    // 获取足够的历史数据
    startTime := time.Now().Add(-time.Duration(periods*5) * time.Minute).Unix()
    endTime := time.Now().Unix()
    
    ticks, err := v1.GetTicks5Minute(ctx, symbol, "binance", "spot", startTime, endTime)
    if err != nil {
        return nil, err
    }
    
    if len(ticks) < periods {
        return nil, fmt.Errorf("数据不足，需要至少 %d 个数据点", periods)
    }
    
    // 计算移动平均线
    var ma []float64
    for i := periods - 1; i < len(ticks); i++ {
        sum := 0.0
        for j := 0; j < periods; j++ {
            price := (ticks[i-j].BestBidPx + ticks[i-j].BestAskPx) / 2
            sum += price
        }
        ma = append(ma, sum/float64(periods))
    }
    
    return ma, nil
}
```

### 3. 数据可视化

```go
// 为图表准备数据
func prepareChartData(ctx context.Context, symbol string, duration time.Duration) ([]ChartPoint, error) {
    var interval pb.Interval
    var limit int32
    
    // 根据时间范围选择合适的间隔
    switch {
    case duration <= time.Hour:
        interval = pb.Interval_INTERVAL_1S
        limit = 3600 // 1小时 = 3600秒
    case duration <= 24*time.Hour:
        interval = pb.Interval_INTERVAL_1M
        limit = 1440 // 24小时 = 1440分钟
    case duration <= 7*24*time.Hour:
        interval = pb.Interval_INTERVAL_5M
        limit = 2016 // 7天 = 2016个5分钟
    default:
        interval = pb.Interval_INTERVAL_1H
        limit = 168 // 7天 = 168小时
    }
    
    startTime := time.Now().Add(-duration).Unix()
    endTime := time.Now().Unix()
    
    ticks, err := v1.GetTicksWithInterval(ctx, symbol, "binance", "spot",
        startTime, endTime, interval, limit, 0)
    if err != nil {
        return nil, err
    }
    
    // 转换为图表数据点
    var points []ChartPoint
    for _, tick := range ticks {
        points = append(points, ChartPoint{
            Time:  time.Unix(int64(tick.ReceiveTime), 0),
            Open:  tick.BestBidPx,
            High:  tick.BestAskPx,
            Low:   tick.BestBidPx,
            Close: (tick.BestBidPx + tick.BestAskPx) / 2,
        })
    }
    
    return points, nil
}

type ChartPoint struct {
    Time  time.Time
    Open  float64
    High  float64
    Low   float64
    Close float64
}
```

### 4. 批量数据处理

```go
// 批量获取多个交易对的数据
func batchGetData(ctx context.Context, symbols []string) (map[string][]*pb.Tick, error) {
    results := make(map[string][]*pb.Tick)
    
    for _, symbol := range symbols {
        ticks, err := v1.GetTicks1Minute(ctx, symbol, "binance", "spot",
            time.Now().Add(-1*time.Hour).Unix(), time.Now().Unix())
        if err != nil {
            log.Printf("获取 %s 数据失败: %v", symbol, err)
            continue
        }
        results[symbol] = ticks
    }
    
    return results, nil
}
```

## 性能优化建议

### 1. 合理选择时间间隔

- **高频交易**: 使用 1ms-100ms 间隔
- **实时监控**: 使用 1s-5s 间隔
- **技术分析**: 使用 1m-15m 间隔
- **长期分析**: 使用 1h-1d 间隔

### 2. 控制数据量

```go
// 根据时间范围自动调整限制
func getOptimalLimit(duration time.Duration, interval pb.Interval) int32 {
    switch interval {
    case pb.Interval_INTERVAL_1S:
        return int32(duration.Seconds())
    case pb.Interval_INTERVAL_1M:
        return int32(duration.Minutes())
    case pb.Interval_INTERVAL_1H:
        return int32(duration.Hours())
    default:
        return 1000 // 默认最大限制
    }
}
```

### 3. 使用分页

```go
// 分页获取大量数据
func getLargeDataset(ctx context.Context, symbol string, totalLimit int) ([]*pb.Tick, error) {
    var allTicks []*pb.Tick
    pageSize := 1000
    offset := 0
    
    for len(allTicks) < totalLimit {
        remaining := totalLimit - len(allTicks)
        if remaining > pageSize {
            remaining = pageSize
        }
        
        ticks, err := v1.GetTicksWithInterval(ctx, symbol, "binance", "spot",
            time.Now().Add(-24*time.Hour).Unix(), time.Now().Unix(),
            pb.Interval_INTERVAL_1M, int32(remaining), int32(offset))
        if err != nil {
            return nil, err
        }
        
        if len(ticks) == 0 {
            break // 没有更多数据
        }
        
        allTicks = append(allTicks, ticks...)
        offset += len(ticks)
    }
    
    return allTicks, nil
}
```

## 错误处理

```go
func robustGetTicks(ctx context.Context, symbol string) ([]*pb.Tick, error) {
    // 重试机制
    for i := 0; i < 3; i++ {
        ticks, err := v1.GetTicks1Minute(ctx, symbol, "binance", "spot",
            time.Now().Add(-1*time.Hour).Unix(), time.Now().Unix())
        if err == nil {
            return ticks, nil
        }
        
        log.Printf("第 %d 次尝试失败: %v", i+1, err)
        if i < 2 {
            time.Sleep(time.Duration(i+1) * time.Second)
        }
    }
    
    return nil, fmt.Errorf("重试3次后仍然失败")
}
```

## 注意事项

1. **数据精度**: 采样数据使用 `argMax` 函数获取每个时间间隔内的最新数据
2. **时间对齐**: 采样时间会自动对齐到间隔的起始点
3. **性能影响**: 较小的间隔会产生更多的数据点，可能影响查询性能
4. **存储空间**: 采样数据不会减少存储空间，只是查询时的数据聚合
5. **实时性**: 采样数据可能有轻微延迟，取决于数据写入频率

通过合理使用时间间隔采样功能，您可以高效地获取和分析不同时间维度的行情数据。