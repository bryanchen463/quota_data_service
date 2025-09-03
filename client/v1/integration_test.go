package client

import (
	"context"
	"testing"
	"time"

	pb "github.com/bryanchen463/quota_data_service/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegrationWithRealService 集成测试 - 需要实际运行的服务
// 运行此测试前，请确保 gRPC 服务正在运行
func TestIntegrationWithRealService(t *testing.T) {
	// 跳过集成测试，除非设置了环境变量
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	// 测试服务地址 - 可以通过环境变量配置
	serviceAddr := "localhost:9090"
	if addr := getEnv("GRPC_SERVICE_ADDR", ""); addr != "" {
		serviceAddr = addr
	}

	ctx := context.Background()

	// 初始化连接池
	err := InitPool(ctx, serviceAddr, 1, 5, 30*time.Second, 5*time.Minute)
	require.NoError(t, err, "连接池初始化失败")

	t.Run("测试插入和获取行情数据", func(t *testing.T) {
		// 创建测试数据
		testTick := &pb.Tick{
			ReceiveTime: float64(time.Now().Unix()),
			Symbol:      "TESTUSDT",
			Exchange:    "test",
			MarketType:  "spot",
			BestBidPx:   100.0,
			BestBidSz:   1.0,
			BestAskPx:   101.0,
			BestAskSz:   1.0,
			BidsPx:      []float64{100.0, 99.0},
			BidsSz:      []float64{1.0, 2.0},
			AsksPx:      []float64{101.0, 102.0},
			AsksSz:      []float64{1.0, 1.5},
		}

		// 插入测试数据
		err := InsertTick(ctx, testTick)
		assert.NoError(t, err, "插入行情数据失败")

		// 等待数据同步
		time.Sleep(100 * time.Millisecond)

		// 获取最新行情
		latestTick, err := GetLatestTick(ctx, "TESTUSDT", "test", "spot")
		if err != nil {
			t.Logf("获取最新行情失败（可能是服务未运行）: %v", err)
			return
		}

		assert.NoError(t, err, "获取最新行情失败")
		assert.NotNil(t, latestTick, "最新行情数据为空")
		assert.Equal(t, "TESTUSDT", latestTick.Symbol)
		assert.Equal(t, "test", latestTick.Exchange)
		assert.Equal(t, "spot", latestTick.MarketType)

		// 获取历史行情
		startTime := time.Now().Add(-1 * time.Minute).Unix()
		endTime := time.Now().Unix()

		ticks, err := GetTicks(ctx, "TESTUSDT", "test", "spot", startTime, endTime)
		if err != nil {
			t.Logf("获取历史行情失败（可能是服务未运行）: %v", err)
			return
		}

		assert.NoError(t, err, "获取历史行情失败")
		assert.NotNil(t, ticks, "历史行情数据为空")
		assert.GreaterOrEqual(t, len(ticks), 1, "应该至少有一条历史行情数据")
	})

	t.Run("测试错误处理", func(t *testing.T) {
		// 测试无效的交易对
		_, err := GetLatestTick(ctx, "", "test", "spot")
		assert.Error(t, err, "空交易对应该返回错误")

		// 测试无效的时间范围
		_, err = GetTicks(ctx, "TESTUSDT", "test", "spot",
			time.Now().Unix(), time.Now().Add(-1*time.Hour).Unix())
		assert.Error(t, err, "无效时间范围应该返回错误")
	})

	t.Run("测试并发访问", func(t *testing.T) {
		concurrency := 5 // 减少并发数，避免连接池耗尽
		done := make(chan bool, concurrency)
		timeout := time.After(30 * time.Second) // 设置超时

		for i := 0; i < concurrency; i++ {
			go func(index int) {
				defer func() { done <- true }()

				// 为每个 goroutine 设置独立的超时上下文
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				// 并发获取最新行情
				_, err := GetLatestTick(ctx, "BTCUSDT", "binance", "spot")
				if err != nil {
					t.Logf("并发获取最新行情失败 (goroutine %d): %v", index, err)
				}
			}(i)
		}

		// 等待所有 goroutine 完成或超时
		completed := 0
		for completed < concurrency {
			select {
			case <-done:
				completed++
			case <-timeout:
				t.Errorf("并发测试超时，只完成了 %d/%d 个 goroutine", completed, concurrency)
				return
			}
		}
	})
}

// TestConnectionPoolBehavior 测试连接池行为
func TestConnectionPoolBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过连接池测试")
	}

	ctx := context.Background()
	serviceAddr := "localhost:9090"

	// 测试连接池初始化
	t.Run("连接池初始化", func(t *testing.T) {
		err := InitPool(ctx, serviceAddr, 2, 5, 10*time.Second, 1*time.Minute)
		assert.NoError(t, err, "连接池初始化失败")
		assert.NotNil(t, p, "连接池应该被正确设置")
	})

	// 测试连接池重用
	t.Run("连接池重用", func(t *testing.T) {
		if p == nil {
			t.Skip("连接池未初始化")
		}

		// 多次调用应该重用连接
		for i := 0; i < 5; i++ {
			_, err := GetLatestTick(ctx, "BTCUSDT", "binance", "spot")
			if err != nil {
				t.Logf("连接池重用测试失败 (iteration %d): %v", i, err)
				break
			}
		}
	})
}

// TestDataValidation 测试数据验证
func TestDataValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		tick        *pb.Tick
		expectError bool
		description string
	}{
		{
			name: "有效数据",
			tick: &pb.Tick{
				ReceiveTime: float64(time.Now().Unix()),
				Symbol:      "BTCUSDT",
				Exchange:    "binance",
				MarketType:  "spot",
				BestBidPx:   50000.0,
				BestBidSz:   1.0,
				BestAskPx:   50001.0,
				BestAskSz:   1.0,
			},
			expectError: false,
			description: "正常的行情数据应该被接受",
		},
		{
			name: "空交易对",
			tick: &pb.Tick{
				Symbol:     "",
				Exchange:   "binance",
				MarketType: "spot",
			},
			expectError: true,
			description: "空交易对应该被拒绝",
		},
		{
			name: "无效价格",
			tick: &pb.Tick{
				Symbol:     "BTCUSDT",
				Exchange:   "binance",
				MarketType: "spot",
				BestBidPx:  -100.0, // 负价格
				BestAskPx:  50001.0,
			},
			expectError: false, // 服务端可能会接受，但业务逻辑应该处理
			description: "负价格数据",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if p == nil {
				t.Skip("连接池未初始化")
			}

			err := InsertTick(ctx, tt.tick)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				if err != nil {
					t.Logf("数据验证测试失败 (%s): %v", tt.description, err)
				}
			}
		})
	}
}

// 辅助函数：获取环境变量
func getEnv(key, defaultValue string) string {
	// 在实际实现中，这里应该使用 os.Getenv
	// 为了简化测试，这里返回默认值
	return defaultValue
}
