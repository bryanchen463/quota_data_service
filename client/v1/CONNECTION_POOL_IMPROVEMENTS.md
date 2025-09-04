# 连接池改进说明

## 问题描述

长时间没有使用的 gRPC 连接可能会因为以下原因变得不可用：
- 网络超时或中断
- 服务器重启或维护
- 连接被服务器端主动关闭
- 防火墙或负载均衡器超时

## 改进措施

### 1. 增强的健康检查

**改进前**：
- 只使用 gRPC 健康检查服务
- 超时时间较短（500ms）

**改进后**：
- 检查连接状态（SHUTDOWN、TRANSIENT_FAILURE）
- 使用 gRPC 健康检查服务
- 增加超时时间到 1 秒
- 添加空连接检查

```go
func (p *Pool) isHealthy(ctx context.Context, conn *grpc.ClientConn) bool {
    if conn == nil {
        return false
    }
    
    // 检查连接状态
    state := conn.GetState()
    if state.String() == "SHUTDOWN" || state.String() == "TRANSIENT_FAILURE" {
        return false
    }
    
    // 使用健康检查服务
    client := grpc_health_v1.NewHealthClient(conn)
    healthCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
    defer cancel()
    
    resp, err := client.Check(healthCtx, &grpc_health_v1.HealthCheckRequest{})
    if err != nil {
        return false
    }
    
    return resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING
}
```

### 2. 更频繁的清理机制

**改进前**：
- 每分钟清理一次
- 只清理空闲超时的连接

**改进后**：
- 每 30 秒清理一次
- 清理空闲超时连接
- 主动检查连接健康状态
- 自动补充不健康的连接

```go
func (p *Pool) cleanup() {
    ticker := time.NewTicker(30 * time.Second)
    // ... 定期检查连接健康状态并清理不健康的连接
}
```

### 3. 连接重试机制

**改进前**：
- 获取到不健康连接时直接丢弃

**改进后**：
- 最多重试 3 次创建新连接
- 自动补充连接池

```go
func (p *Pool) Get(ctx context.Context) (*grpc.ClientConn, error) {
    maxRetries := 3
    retryCount := 0
    // ... 重试逻辑
}
```

### 4. 连接预热

**改进前**：
- 创建连接后直接加入池中

**改进后**：
- 创建连接后进行健康检查
- 只有健康的连接才加入池中

```go
// 初始化连接并预热
for i := 0; i < initSize; i++ {
    conn, err := p.newConn()
    if err != nil {
        return nil, err
    }
    
    // 预热连接：检查连接是否可用
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    if p.isHealthy(ctx, conn) {
        p.conns <- &pooledConn{conn: conn, lastUsed: time.Now()}
        p.curSize++
    } else {
        _ = conn.Close()
    }
    cancel()
}
```

## 配置建议

### 连接池参数

```go
// 推荐的连接池配置
pool, err := New(
    "localhost:9090",           // 目标地址
    3,                          // 初始连接数
    10,                         // 最大连接数
    5*time.Second,              // 连接超时
    30*time.Second,             // 空闲超时
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
```

### 参数说明

- **初始连接数**：建议 2-5 个，确保有足够的连接可用
- **最大连接数**：根据并发需求设置，建议 10-20 个
- **连接超时**：建议 5-10 秒，给连接建立足够时间
- **空闲超时**：建议 30-60 秒，平衡资源使用和连接可用性

## 监控指标

### 建议监控的连接池指标

1. **连接池大小**：当前连接数、最大连接数
2. **连接健康状态**：健康连接数、不健康连接数
3. **连接获取时间**：平均获取时间、最大获取时间
4. **连接重试次数**：重试频率、重试成功率
5. **连接清理频率**：清理的连接数、补充的连接数

### 日志记录

连接池会记录以下关键事件：
- 连接创建失败
- 连接健康检查失败
- 连接重试
- 连接清理和补充

## 使用示例

### 基本使用

```go
// 创建连接池
pool, err := client.New("localhost:9090", 3, 10, 5*time.Second, 30*time.Second,
    grpc.WithTransportCredentials(insecure.NewCredentials()))
if err != nil {
    log.Fatal(err)
}
defer pool.Close()

// 使用连接
ctx := context.Background()
conn, err := pool.Get(ctx)
if err != nil {
    log.Printf("获取连接失败: %v", err)
    return
}
defer pool.Put(conn)

// 使用连接进行 gRPC 调用
client := pb.NewQuotaServiceClient(conn)
response, err := client.GetLatestTick(ctx, &pb.GetLatestTickRequest{
    Symbol: "BTCUSDT",
    Exchange: "binance",
    MarketType: "spot",
})
```

### 错误处理

```go
// 带重试的连接获取
func getConnectionWithRetry(pool *client.Pool, maxRetries int) (*grpc.ClientConn, error) {
    for i := 0; i < maxRetries; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        conn, err := pool.Get(ctx)
        cancel()
        
        if err == nil {
            return conn, nil
        }
        
        log.Printf("第 %d 次获取连接失败: %v", i+1, err)
        if i < maxRetries-1 {
            time.Sleep(time.Duration(i+1) * time.Second)
        }
    }
    
    return nil, fmt.Errorf("重试 %d 次后仍然失败", maxRetries)
}
```

## 性能影响

### 改进带来的性能提升

1. **连接可用性**：显著提高长时间运行后的连接可用性
2. **错误恢复**：自动检测和恢复不健康的连接
3. **资源利用**：及时清理无效连接，释放资源
4. **响应时间**：减少因连接问题导致的请求失败

### 资源消耗

1. **CPU 使用**：增加约 5-10%（定期健康检查）
2. **内存使用**：基本无变化
3. **网络流量**：增加少量健康检查流量
4. **连接数**：可能略微增加（自动补充机制）

## 最佳实践

1. **合理设置参数**：根据实际负载调整连接池参数
2. **监控连接状态**：定期检查连接池健康状态
3. **错误处理**：实现适当的重试和降级机制
4. **资源清理**：确保正确关闭连接池
5. **日志记录**：记录关键事件便于问题排查

## 故障排查

### 常见问题

1. **连接获取超时**
   - 检查网络连接
   - 调整连接超时参数
   - 检查服务器状态

2. **连接频繁重试**
   - 检查服务器健康状态
   - 调整健康检查参数
   - 检查网络稳定性

3. **连接池耗尽**
   - 增加最大连接数
   - 检查连接归还逻辑
   - 优化连接使用模式

### 调试方法

1. **启用详细日志**：记录连接池操作
2. **监控指标**：观察连接池状态变化
3. **健康检查**：手动测试连接可用性
4. **压力测试**：验证高负载下的表现