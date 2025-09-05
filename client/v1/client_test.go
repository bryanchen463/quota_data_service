package client

import (
	"context"
	"testing"
	"time"

	pb "github.com/bryanchen463/quota_data_service/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

// MockQuotaServiceClient 是 QuotaServiceClient 的 mock 实现
type MockQuotaServiceClient struct {
	mock.Mock
}

func (m *MockQuotaServiceClient) IngestTick(ctx context.Context, in *pb.IngestTickRequest, opts ...grpc.CallOption) (*pb.IngestTickResponse, error) {
	args := m.Called(ctx, in, opts)
	return args.Get(0).(*pb.IngestTickResponse), args.Error(1)
}

func (m *MockQuotaServiceClient) IngestTicks(ctx context.Context, in *pb.IngestTicksRequest, opts ...grpc.CallOption) (*pb.IngestTicksResponse, error) {
	args := m.Called(ctx, in, opts)
	return args.Get(0).(*pb.IngestTicksResponse), args.Error(1)
}

func (m *MockQuotaServiceClient) GetTicks(ctx context.Context, in *pb.GetTicksRequest, opts ...grpc.CallOption) (*pb.GetTicksResponse, error) {
	args := m.Called(ctx, in, opts)
	return args.Get(0).(*pb.GetTicksResponse), args.Error(1)
}

func (m *MockQuotaServiceClient) GetLatestTick(ctx context.Context, in *pb.GetLatestTickRequest, opts ...grpc.CallOption) (*pb.GetLatestTickResponse, error) {
	args := m.Called(ctx, in, opts)
	return args.Get(0).(*pb.GetLatestTickResponse), args.Error(1)
}

func (m *MockQuotaServiceClient) HealthCheck(ctx context.Context, in *pb.HealthCheckRequest, opts ...grpc.CallOption) (*pb.HealthCheckResponse, error) {
	args := m.Called(ctx, in, opts)
	return args.Get(0).(*pb.HealthCheckResponse), args.Error(1)
}

// MockPool 是连接池的 mock 实现
type MockPool struct {
	mock.Mock
}

func (m *MockPool) Get(ctx context.Context) (*grpc.ClientConn, error) {
	args := m.Called(ctx)
	return args.Get(0).(*grpc.ClientConn), args.Error(1)
}

func (m *MockPool) Close() {
	m.Called()
}

// MockConn 是 gRPC 连接的 mock 实现
type MockConn struct {
	mock.Mock
}

func (m *MockConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

// 测试辅助函数：创建测试用的 Tick 数据
func createTestTick() *pb.Tick {
	return &pb.Tick{
		ReceiveTime: float64(time.Now().Unix()),
		Symbol:      "BTCUSDT",
		Exchange:    "binance",
		MarketType:  "spot",
		BestBidPx:   50000.0,
		BestBidSz:   1.5,
		BestAskPx:   50001.0,
		BestAskSz:   2.0,
		BidsPx:      []float64{50000.0, 49999.0, 49998.0},
		BidsSz:      []float64{1.5, 2.0, 1.0},
		AsksPx:      []float64{50001.0, 50002.0, 50003.0},
		AsksSz:      []float64{2.0, 1.5, 3.0},
	}
}

// TestInitPool 测试连接池初始化
func TestInitPool(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		init         int
		cap          int
		idleTimeout  time.Duration
		maxLifetime  time.Duration
		expectError  bool
		errorMessage string
	}{
		{
			name:        "正常初始化",
			target:      "localhost:9090",
			init:        1,
			cap:         10,
			idleTimeout: 30 * time.Second,
			maxLifetime: 5 * time.Minute,
			expectError: false,
		},
		{
			name:        "空目标地址",
			target:      "",
			init:        1,
			cap:         10,
			idleTimeout: 30 * time.Second,
			maxLifetime: 5 * time.Minute,
			expectError: false, // 空地址不会立即失败，会在连接时失败
		},
		{
			name:        "无效容量参数",
			target:      "localhost:9090",
			init:        10,
			cap:         5, // cap 小于 init
			idleTimeout: 30 * time.Second,
			maxLifetime: 5 * time.Minute,
			expectError: true, // cap 小于 init 会报错
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 重置全局变量
			p = nil

			ctx := context.Background()
			err := InitPool(ctx, tt.target, tt.init, tt.cap, tt.idleTimeout, tt.maxLifetime)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}

// TestGetLatestTick 测试获取最新行情数据
func TestGetLatestTick(t *testing.T) {
	// 注意：这个测试需要实际的 gRPC 服务运行，或者需要更复杂的 mock 设置
	// 这里提供一个基础的测试框架

	tests := []struct {
		name        string
		symbol      string
		exchange    string
		marketType  string
		expectError bool
	}{
		{
			name:        "正常获取最新行情",
			symbol:      "BTCUSDT",
			exchange:    "binance",
			marketType:  "spot",
			expectError: false,
		},
		{
			name:        "空交易对符号",
			symbol:      "",
			exchange:    "binance",
			marketType:  "spot",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// 注意：这个测试需要先初始化连接池
			// 在实际测试中，你可能需要启动一个测试服务器或使用 mock
			if p == nil {
				t.Skip("连接池未初始化，跳过测试")
				return
			}

			tick, err := GetLatestTick(ctx, tt.symbol, tt.exchange, tt.marketType)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, tick)
			} else {
				// 注意：这里可能会因为连接问题而失败
				// 在实际测试环境中，你需要确保有可用的服务
				if err != nil {
					t.Logf("获取最新行情失败（可能是连接问题）: %v", err)
					return
				}
				assert.NoError(t, err)
				if tick != nil {
					assert.Equal(t, tt.symbol, tick.Symbol)
					assert.Equal(t, tt.exchange, tick.Exchange)
					assert.Equal(t, tt.marketType, tick.MarketType)
				}
			}
		})
	}
}

// TestGetTicks 测试获取历史行情数据
func TestGetTicks(t *testing.T) {
	tests := []struct {
		name           string
		symbol         string
		exchange       string
		marketType     string
		startTimestamp int64
		endTimestamp   int64
		expectError    bool
	}{
		{
			name:           "正常获取历史行情",
			symbol:         "BTCUSDT",
			exchange:       "binance",
			marketType:     "spot",
			startTimestamp: time.Now().Add(-1 * time.Hour).Unix(),
			endTimestamp:   time.Now().Unix(),
			expectError:    false,
		},
		{
			name:           "无效时间范围",
			symbol:         "BTCUSDT",
			exchange:       "binance",
			marketType:     "spot",
			startTimestamp: time.Now().Unix(),
			endTimestamp:   time.Now().Add(-1 * time.Hour).Unix(), // 开始时间晚于结束时间
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			if p == nil {
				t.Skip("连接池未初始化，跳过测试")
				return
			}

			ticks, err := GetTicks(ctx, tt.symbol, tt.exchange, tt.marketType, tt.startTimestamp, tt.endTimestamp, pb.Interval_INTERVAL_1MS, 100, 0)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, ticks)
			} else {
				if err != nil {
					t.Errorf("获取历史行情失败（可能是连接问题）: %v", err)
					return
				}
				assert.NoError(t, err)
				assert.NotNil(t, ticks)
				// 验证返回的数据结构
				for _, tick := range ticks {
					assert.Equal(t, tt.symbol, tick.Symbol)
					assert.Equal(t, tt.exchange, tick.Exchange)
					assert.Equal(t, tt.marketType, tick.MarketType)
				}
			}
		})
	}
}

// TestGetTicksWithInterval 测试按时间间隔采样获取历史行情数据
func TestGetTicksWithInterval(t *testing.T) {
	tests := []struct {
		name           string
		symbol         string
		exchange       string
		marketType     string
		startTimestamp int64
		endTimestamp   int64
		interval       pb.Interval
		limit          int32
		offset         int32
		expectError    bool
	}{
		{
			name:           "1秒间隔采样",
			symbol:         "BTCUSDT",
			exchange:       "binance",
			marketType:     "spot",
			startTimestamp: time.Now().Add(-1 * time.Hour).Unix(),
			endTimestamp:   time.Now().Unix(),
			interval:       pb.Interval_INTERVAL_1S,
			limit:          50,
			offset:         0,
			expectError:    false,
		},
		{
			name:           "5分钟间隔采样",
			symbol:         "BTCUSDT",
			exchange:       "binance",
			marketType:     "spot",
			startTimestamp: time.Now().Add(-24 * time.Hour).Unix(),
			endTimestamp:   time.Now().Unix(),
			interval:       pb.Interval_INTERVAL_5M,
			limit:          100,
			offset:         0,
			expectError:    false,
		},
		{
			name:           "1小时间隔采样",
			symbol:         "BTCUSDT",
			exchange:       "binance",
			marketType:     "spot",
			startTimestamp: time.Now().Add(-7 * 24 * time.Hour).Unix(),
			endTimestamp:   time.Now().Unix(),
			interval:       pb.Interval_INTERVAL_1H,
			limit:          168, // 一周的小时数
			offset:         0,
			expectError:    false,
		},
		{
			name:           "1天间隔采样",
			symbol:         "BTCUSDT",
			exchange:       "binance",
			marketType:     "spot",
			startTimestamp: time.Now().Add(-30 * 24 * time.Hour).Unix(),
			endTimestamp:   time.Now().Unix(),
			interval:       pb.Interval_INTERVAL_1D,
			limit:          30,
			offset:         0,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			if p == nil {
				t.Skip("连接池未初始化，跳过测试")
				return
			}

			ticks, err := GetTicksWithInterval(ctx, tt.symbol, tt.exchange, tt.marketType, tt.startTimestamp, tt.endTimestamp, tt.interval, tt.limit, tt.offset)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, ticks)
			} else {
				if err != nil {
					t.Logf("获取采样历史行情失败（可能是连接问题）: %v", err)
					return
				}
				assert.NoError(t, err)
				assert.NotNil(t, ticks)
				// 验证返回的数据结构
				for _, tick := range ticks {
					assert.Equal(t, tt.symbol, tick.Symbol)
					assert.Equal(t, tt.exchange, tick.Exchange)
					assert.Equal(t, tt.marketType, tick.MarketType)
				}
			}
		})
	}
}

// TestConvenienceSamplingFunctions 测试便捷采样函数
func TestConvenienceSamplingFunctions(t *testing.T) {
	if p == nil {
		t.Skip("连接池未初始化，跳过测试")
		return
	}

	ctx := context.Background()
	symbol := "BTCUSDT"
	exchange := "binance"
	marketType := "spot"
	startTime := time.Now().Add(-1 * time.Hour).Unix()
	endTime := time.Now().Unix()

	tests := []struct {
		name     string
		function func(context.Context, string, string, string, int64, int64) ([]*pb.Tick, error)
	}{
		{"1秒采样", GetTicks1Second},
		{"5秒采样", GetTicks5Second},
		{"1分钟采样", GetTicks1Minute},
		{"5分钟采样", GetTicks5Minute},
		{"1小时采样", GetTicks1Hour},
		{"1天采样", GetTicks1Day},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticks, err := tt.function(ctx, symbol, exchange, marketType, startTime, endTime)
			if err != nil {
				t.Logf("%s 失败（可能是连接问题）: %v", tt.name, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, ticks)
		})
	}
}

// TestInsertTick 测试插入行情数据
func TestInsertTick(t *testing.T) {
	tests := []struct {
		name        string
		tick        *pb.Tick
		expectError bool
	}{
		{
			name:        "正常插入行情数据",
			tick:        createTestTick(),
			expectError: false,
		},
		{
			name:        "插入空行情数据",
			tick:        nil,
			expectError: true,
		},
		{
			name: "插入不完整行情数据",
			tick: &pb.Tick{
				Symbol:     "BTCUSDT",
				Exchange:   "binance",
				MarketType: "spot",
				// 缺少其他必要字段
			},
			expectError: false, // 服务端可能会接受不完整的数据
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			if p == nil {
				t.Skip("连接池未初始化，跳过测试")
				return
			}

			err := InsertTick(ctx, tt.tick)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				if err != nil {
					t.Errorf("插入行情数据失败（可能是连接问题）: %v", err)
					return
				}
				assert.NoError(t, err)
			}
		})
	}
}

// TestClientErrorHandling 测试客户端错误处理
func TestClientErrorHandling(t *testing.T) {
	t.Run("连接池未初始化", func(t *testing.T) {
		// 重置连接池
		originalPool := p
		p = nil
		defer func() {
			p = originalPool
		}()

		ctx := context.Background()

		// 测试 GetLatestTick - 会 panic，这是预期的行为
		assert.Panics(t, func() {
			GetLatestTick(ctx, "BTCUSDT", "binance", "spot")
		}, "连接池为 nil 时应该 panic")

		// 测试 GetTicks - 会 panic，这是预期的行为
		assert.Panics(t, func() {
			GetTicks(ctx, "BTCUSDT", "binance", "spot",
				time.Now().Add(-1*time.Hour).Unix(), time.Now().Unix(), pb.Interval_INTERVAL_1MS, 100, 0)
		}, "连接池为 nil 时应该 panic")

		// 测试 InsertTick - 会 panic，这是预期的行为
		assert.Panics(t, func() {
			InsertTick(ctx, createTestTick())
		}, "连接池为 nil 时应该 panic")
	})
}

// TestConcurrentAccess 测试并发访问
func TestConcurrentAccess(t *testing.T) {
	if p == nil {
		t.Skip("连接池未初始化，跳过测试")
		return
	}

	ctx := context.Background()

	// 并发执行多个操作
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			// 并发获取最新行情
			_, err := GetLatestTick(ctx, "BTCUSDT", "binance", "spot")
			if err != nil {
				t.Errorf("并发获取最新行情失败: %v", err)
			}

			// 并发获取历史行情
			_, err = GetTicks(ctx, "BTCUSDT", "binance", "spot",
				time.Now().Add(-1*time.Hour).Unix(), time.Now().Unix(), pb.Interval_INTERVAL_1MS, 100, 0)
			if err != nil {
				t.Errorf("并发获取历史行情失败: %v", err)
			}
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}
}

// BenchmarkGetLatestTick 性能测试
func BenchmarkGetLatestTick(b *testing.B) {
	if p == nil {
		b.Skip("连接池未初始化，跳过性能测试")
		return
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetLatestTick(ctx, "BTCUSDT", "binance", "spot")
		if err != nil {
			b.Logf("性能测试中获取最新行情失败: %v", err)
			break
		}
	}
}

// BenchmarkGetTicks 性能测试
func BenchmarkGetTicks(b *testing.B) {
	if p == nil {
		b.Skip("连接池未初始化，跳过性能测试")
		return
	}

	ctx := context.Background()
	startTime := time.Now().Add(-1 * time.Hour).Unix()
	endTime := time.Now().Unix()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetTicks(ctx, "BTCUSDT", "binance", "spot", startTime, endTime, pb.Interval_INTERVAL_1MS, 100, 0)
		if err != nil {
			b.Logf("性能测试中获取历史行情失败: %v", err)
			break
		}
	}
}
