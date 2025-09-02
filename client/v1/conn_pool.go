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

	// 初始化连接
	for i := 0; i < initSize; i++ {
		conn, err := p.newConn()
		if err != nil {
			return nil, err
		}
		p.conns <- &pooledConn{conn: conn, lastUsed: time.Now()}
		p.curSize++
	}

	// 启动后台清理协程
	p.wg.Add(1)
	go p.cleanup()

	return p, nil
}

// Get 从池中获取一个连接（带超时）
func (p *Pool) Get(ctx context.Context) (*grpc.ClientConn, error) {
	for {
		select {
		case pc := <-p.conns:
			if !p.isHealthy(ctx, pc.conn) {
				_ = pc.conn.Close()
				p.mu.Lock()
				p.curSize--
				p.mu.Unlock()
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

		// 阻塞等待可用连接
		select {
		case pc := <-p.conns:
			if !p.isHealthy(ctx, pc.conn) {
				_ = pc.conn.Close()
				p.mu.Lock()
				p.curSize--
				p.mu.Unlock()
				continue
			}
			pc.lastUsed = time.Now()
			return pc.conn, nil
		case <-ctx.Done():
			return nil, ctx.Err()
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
	return grpc.NewClient(p.target, p.opts...)
}

// isHealthy 检查连接是否健康
func (p *Pool) isHealthy(ctx context.Context, conn *grpc.ClientConn) bool {
	client := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	return err == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING
}

// cleanup 定期清理空闲超时的连接
func (p *Pool) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer func() {
		ticker.Stop()
		p.wg.Done()
	}()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			n := len(p.conns)
			for i := 0; i < n; i++ {
				select {
				case pc := <-p.conns:
					if time.Since(pc.lastUsed) > p.idleTimeout && p.curSize > p.initSize {
						_ = pc.conn.Close()
						p.curSize--
					} else {
						p.conns <- pc // 放回去
					}
				default:
				}
			}
			p.mu.Unlock()
		case <-p.stopCh:
			return
		}
	}
}
