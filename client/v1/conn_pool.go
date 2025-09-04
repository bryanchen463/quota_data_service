package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type pooledConn struct {
	conn     *grpc.ClientConn
	lastUsed time.Time
}

type Pool struct {
	target      string
	opts        []grpc.DialOption
	conns       chan *pooledConn
	mu          sync.Mutex
	maxSize     int
	curSize     int
	initSize    int
	dialTimeout time.Duration
	idleTimeout time.Duration
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// New 创建一个新的 gRPC 连接池
func New(target string, initSize, maxSize int, dialTimeout, idleTimeout time.Duration, opts ...grpc.DialOption) (*Pool, error) {
	if initSize <= 0 || maxSize <= 0 || initSize > maxSize {
		return nil, errors.New("invalid pool size config")
	}

	p := &Pool{
		target:      target,
		opts:        opts,
		conns:       make(chan *pooledConn, maxSize),
		maxSize:     maxSize,
		initSize:    initSize,
		dialTimeout: dialTimeout,
		idleTimeout: idleTimeout,
		stopCh:      make(chan struct{}),
	}

	// 初始化连接并预热
	for i := 0; i < initSize; i++ {
		conn, err := p.newConn()
		if err != nil {
			// 如果无法创建连接，记录错误但继续尝试其他连接
			continue
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

	// 启动后台清理协程
	p.wg.Add(1)
	go p.cleanup()

	return p, nil
}

// Get 从池中获取一个连接（带超时和重试机制）
func (p *Pool) Get(ctx context.Context) (*grpc.ClientConn, error) {
	maxRetries := 3
	retryCount := 0

	for {
		select {
		case pc := <-p.conns:
			if !p.isHealthy(ctx, pc.conn) {
				_ = pc.conn.Close()
				p.mu.Lock()
				p.curSize--
				p.mu.Unlock()

				// 如果重试次数未达到上限，尝试创建新连接
				if retryCount < maxRetries {
					retryCount++
					conn, err := p.newConn()
					if err == nil {
						p.mu.Lock()
						p.curSize++
						p.mu.Unlock()
						return conn, nil
					}
				}
				continue
			}
			pc.lastUsed = time.Now()
			return pc.conn, nil
		default:
			p.mu.Lock()
			if p.curSize < p.maxSize {
				conn, err := p.newConn()
				if err != nil {
					p.mu.Unlock()
					return nil, err
				}
				p.curSize++
				p.mu.Unlock()
				return conn, nil
			}
			p.mu.Unlock()
		}

		// 阻塞等待可用连接，但添加超时保护
		select {
		case pc := <-p.conns:
			if !p.isHealthy(ctx, pc.conn) {
				_ = pc.conn.Close()
				p.mu.Lock()
				p.curSize--
				p.mu.Unlock()

				// 如果重试次数未达到上限，尝试创建新连接
				if retryCount < maxRetries {
					retryCount++
					conn, err := p.newConn()
					if err == nil {
						p.mu.Lock()
						p.curSize++
						p.mu.Unlock()
						return conn, nil
					}
				}
				continue
			}
			pc.lastUsed = time.Now()
			return pc.conn, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			// 如果等待连接超过5秒，返回超时错误
			return nil, errors.New("timeout waiting for available connection")
		}
	}
}

// Put 归还连接到池中
func (p *Pool) Put(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}
	pc := &pooledConn{conn: conn, lastUsed: time.Now()}
	select {
	case p.conns <- pc:
	default:
		// 池已满，关闭连接
		_ = conn.Close()
		p.mu.Lock()
		p.curSize--
		p.mu.Unlock()
	}
}

// Close 关闭池中的所有连接
func (p *Pool) Close() {
	close(p.stopCh)
	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()
	close(p.conns)
	for pc := range p.conns {
		_ = pc.conn.Close()
	}
	p.curSize = 0
}

func (p *Pool) newConn() (*grpc.ClientConn, error) {
	return grpc.Dial(p.target, p.opts...)
}

// isHealthy 检查连接是否健康
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

// cleanup 定期清理空闲超时的连接和检查连接健康状态
func (p *Pool) cleanup() {
	// 更频繁的清理间隔，每30秒检查一次
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		p.wg.Done()
	}()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			n := len(p.conns)
			unhealthyCount := 0

			for i := 0; i < n; i++ {
				select {
				case pc := <-p.conns:
					// 检查连接是否空闲超时
					if time.Since(pc.lastUsed) > p.idleTimeout && p.curSize > p.initSize {
						_ = pc.conn.Close()
						p.curSize--
						continue
					}

					// 检查连接健康状态（使用短超时避免阻塞）
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					if !p.isHealthy(ctx, pc.conn) {
						_ = pc.conn.Close()
						p.curSize--
						unhealthyCount++
						cancel()
						continue
					}
					cancel()

					// 健康的连接放回池中
					p.conns <- pc
				default:
				}
			}

			// 如果有不健康的连接被清理，尝试补充到初始大小
			if unhealthyCount > 0 && p.curSize < p.initSize {
				for p.curSize < p.initSize {
					conn, err := p.newConn()
					if err != nil {
						break // 如果创建连接失败，停止尝试
					}
					p.conns <- &pooledConn{conn: conn, lastUsed: time.Now()}
					p.curSize++
				}
			}

			p.mu.Unlock()
		case <-p.stopCh:
			return
		}
	}
}
