# 客户端测试文档

本文档描述了如何运行和管理 `client/v1` 包的测试用例。

## 测试结构

### 测试文件

- `client_test.go` - 主要单元测试和 mock 测试
- `integration_test.go` - 集成测试（需要实际运行的服务）
- `test_config.go` - 测试配置管理
- `Makefile` - 测试命令和工具

### 测试类型

1. **单元测试** - 使用 mock 对象，不依赖外部服务
2. **集成测试** - 需要实际运行的 gRPC 服务
3. **性能测试** - 基准测试和并发测试
4. **错误处理测试** - 测试各种错误情况

## 运行测试

### 基本命令

```bash
# 运行所有测试
make test

# 运行单元测试（跳过集成测试）
make test-unit

# 运行集成测试
make test-integration

# 运行性能测试
make test-benchmark

# 生成覆盖率报告
make test-coverage
```

### 环境变量配置

可以通过环境变量配置测试行为：

```bash
# 启用集成测试
export ENABLE_INTEGRATION_TESTS=true

# 设置服务地址
export TEST_GRPC_SERVICE_ADDR=localhost:9090

# 设置连接池参数
export TEST_INIT_CONN=2
export TEST_MAX_CONN=10
export TEST_IDLE_TIMEOUT=30s
export TEST_MAX_LIFETIME=5m

# 启用 mock 模式
export TEST_ENABLE_MOCK=true
export TEST_MOCK_DELAY=100ms
```

### 运行特定测试

```bash
# 运行特定测试函数
make test-specific TEST=TestGetLatestTick

# 使用 go test 直接运行
go test -v -run TestGetLatestTick ./...
```

## 测试用例说明

### InitPool 测试

测试连接池初始化功能：
- 正常初始化
- 无效参数处理
- 错误情况处理

### GetLatestTick 测试

测试获取最新行情数据：
- 正常获取
- 参数验证
- 错误处理
- 并发访问

### GetTicks 测试

测试获取历史行情数据：
- 时间范围查询
- 参数验证
- 数据格式验证
- 分页处理

### InsertTick 测试

测试插入行情数据：
- 数据插入
- 数据验证
- 错误处理
- 批量操作

## Mock 测试

测试使用 mock 对象来模拟 gRPC 服务：

```go
// 创建 mock 客户端
mockClient := &MockQuotaServiceClient{}

// 设置期望行为
mockClient.On("GetLatestTick", mock.Anything, mock.Anything).Return(
    &pb.GetLatestTickResponse{
        Success: true,
        Tick:    testTick,
    }, nil)

// 执行测试
result, err := GetLatestTick(ctx, "BTCUSDT", "binance", "spot")
```

## 集成测试

集成测试需要实际运行的 gRPC 服务：

1. 启动服务：
```bash
# 在项目根目录
go run main.go
```

2. 运行集成测试：
```bash
ENABLE_INTEGRATION_TESTS=true make test-integration
```

## 性能测试

运行性能测试来评估客户端性能：

```bash
# 运行所有基准测试
make test-benchmark

# 运行特定基准测试
go test -bench=BenchmarkGetLatestTick -benchmem ./...
```

## 覆盖率要求

项目要求测试覆盖率达到 80% 以上：

```bash
# 检查覆盖率
make test-coverage-threshold
```

## 故障排除

### 常见问题

1. **连接失败**
   - 确保 gRPC 服务正在运行
   - 检查服务地址和端口
   - 验证网络连接

2. **测试超时**
   - 增加超时时间
   - 检查服务响应时间
   - 优化测试数据量

3. **Mock 测试失败**
   - 检查 mock 对象设置
   - 验证期望参数
   - 确保 mock 方法被正确调用

### 调试技巧

```bash
# 详细输出
go test -v -args -test.v

# 运行单个测试并显示详细输出
go test -v -run TestGetLatestTick -args -test.v

# 使用 race detector
go test -race ./...
```

## 持续集成

在 CI/CD 管道中运行测试：

```yaml
# GitHub Actions 示例
- name: Run Tests
  run: |
    make deps
    make test-unit
    make test-coverage-threshold
```

## 最佳实践

1. **测试隔离** - 每个测试应该独立运行
2. **数据清理** - 测试后清理测试数据
3. **错误处理** - 测试各种错误情况
4. **并发测试** - 验证并发安全性
5. **性能监控** - 定期运行性能测试
6. **覆盖率监控** - 保持高测试覆盖率

## 贡献指南

添加新测试时请遵循以下原则：

1. 测试名称应该清晰描述测试内容
2. 使用表驱动测试处理多个测试用例
3. 添加适当的注释和文档
4. 确保测试可以独立运行
5. 更新相关文档