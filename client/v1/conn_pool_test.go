package client

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestPoolConnectionHealth(t *testing.T) {
	// 注意：这个测试需要实际运行的 gRPC 服务
	if testing.Short() {
		t.Skip("跳过需要实际服务的测试")
	}

	// 创建一个连接池
	pool, err := New("localhost:9090", 2, 5, 5*time.Second, 30*time.Second,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer pool.Close()

	ctx := context.Background()

	// 测试获取连接
	conn, err := pool.Get(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	// 归还连接
	pool.Put(conn)

	// 等待一段时间，让清理协程运行
	time.Sleep(35 * time.Second)

	// 再次获取连接，应该仍然可用
	conn2, err := pool.Get(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn2)

	pool.Put(conn2)
}

func TestPoolConnectionRetry(t *testing.T) {
	// 测试连接重试机制
	pool, err := New("invalid:9999", 1, 3, 1*time.Second, 10*time.Second,
		grpc.WithTransportCredentials(insecure.NewCredentials()))

	// 连接池创建可能失败，这是正常的
	if err != nil {
		t.Logf("连接池创建失败（预期行为）: %v", err)
		return
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 尝试获取连接，可能会成功（连接创建成功）但连接不可用
	conn, err := pool.Get(ctx)
	if err != nil {
		// 如果获取连接失败，这是预期的
		assert.Nil(t, conn)
		return
	}

	// 如果获取到连接，测试连接是否真的可用
	if conn != nil {
		// 尝试使用连接进行健康检查，应该失败
		client := grpc_health_v1.NewHealthClient(conn)
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 1*time.Second)
		_, healthErr := client.Check(healthCtx, &grpc_health_v1.HealthCheckRequest{})
		healthCancel()

		// 健康检查应该失败，因为地址无效
		assert.Error(t, healthErr)
	}
}

func TestPoolConnectionWarmup(t *testing.T) {
	// 测试连接预热
	if testing.Short() {
		t.Skip("跳过需要实际服务的测试")
	}

	pool, err := New("localhost:9090", 3, 5, 5*time.Second, 30*time.Second,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer pool.Close()

	// 连接池应该已经预热了3个连接
	ctx := context.Background()

	// 连续获取多个连接，应该都能成功
	for i := 0; i < 3; i++ {
		conn, err := pool.Get(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, conn)
		pool.Put(conn)
	}
}

func TestPoolCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过需要实际服务的测试")
	}

	// 创建一个短空闲超时的连接池
	pool, err := New("localhost:9090", 1, 3, 5*time.Second, 5*time.Second,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	defer pool.Close()

	ctx := context.Background()

	// 获取连接
	conn, err := pool.Get(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	// 归还连接
	pool.Put(conn)

	// 等待超过空闲超时时间
	time.Sleep(6 * time.Second)

	// 再次获取连接，应该创建新连接
	conn2, err := pool.Get(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn2)

	pool.Put(conn2)
}

// BenchmarkPoolGet 性能测试
func BenchmarkPoolGet(b *testing.B) {
	if testing.Short() {
		b.Skip("跳过需要实际服务的性能测试")
	}

	pool, err := New("localhost:9090", 5, 10, 5*time.Second, 30*time.Second,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatalf("创建连接池失败: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Get(ctx)
			if err != nil {
				b.Errorf("获取连接失败: %v", err)
				continue
			}
			pool.Put(conn)
		}
	})
}
