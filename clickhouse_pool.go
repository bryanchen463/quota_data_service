package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	ch "github.com/ClickHouse/clickhouse-go/v2"
	chlib "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// CHPool ClickHouse 连接池
type CHPool struct {
	mu          sync.RWMutex
	connections chan chlib.Conn
	config      *CHPoolConfig
	closed      bool
	stats       *CHPoolStats
}

// CHPoolConfig 连接池配置
type CHPoolConfig struct {
	// 连接配置
	Addr     string
	User     string
	Password string
	Database string
	Table    string

	// 连接池配置
	MaxConnections int           // 最大连接数
	MinConnections int           // 最小连接数
	MaxIdleTime    time.Duration // 连接最大空闲时间
	MaxLifetime    time.Duration // 连接最大生命周期

	// 连接选项
	DialTimeout             time.Duration
	MaxOpenConns            int
	MaxIdleConns            int
	ConnMaxLifetime         time.Duration
	ConnMaxIdleTime         time.Duration
	Compression             bool
	MaxExecutionTime        time.Duration
	MaxBlockSize            int
	MinInsertBlockSizeRows  int
	MinInsertBlockSizeBytes int
}

// CHPoolStats 连接池统计信息
type CHPoolStats struct {
	TotalConnections  int
	ActiveConnections int
	IdleConnections   int
	TotalQueries      int64
	FailedQueries     int64
	LastQueryTime     time.Time
}

// NewCHPool 创建新的 ClickHouse 连接池
func NewCHPool(config *CHPoolConfig) (*CHPool, error) {
	if config.MaxConnections <= 0 {
		config.MaxConnections = 10
	}
	if config.MinConnections <= 0 {
		config.MinConnections = 2
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = 10 * time.Second
	}
	if config.MaxIdleTime <= 0 {
		config.MaxIdleTime = 5 * time.Minute
	}
	if config.MaxLifetime <= 0 {
		config.MaxLifetime = 30 * time.Minute
	}

	pool := &CHPool{
		connections: make(chan chlib.Conn, config.MaxConnections),
		config:      config,
		stats:       &CHPoolStats{},
	}

	// 创建初始连接
	for i := 0; i < config.MinConnections; i++ {
		conn, err := pool.createConnection()
		if err != nil {
			// 清理已创建的连接
			pool.Close()
			return nil, fmt.Errorf("failed to create initial connection %d: %w", i+1, err)
		}
		pool.connections <- conn
		pool.stats.TotalConnections++
		pool.stats.IdleConnections++
	}

	// 启动连接清理协程
	go pool.cleanupRoutine()

	return pool, nil
}

// createConnection 创建新的 ClickHouse 连接
func (p *CHPool) createConnection() (chlib.Conn, error) {
	opts := &ch.Options{
		Addr: []string{p.config.Addr},
		Auth: ch.Auth{
			Database: p.config.Database,
			Username: p.config.User,
			Password: p.config.Password,
		},
		DialTimeout:     p.config.DialTimeout,
		MaxOpenConns:    p.config.MaxOpenConns,
		MaxIdleConns:    p.config.MaxIdleConns,
		ConnMaxLifetime: p.config.ConnMaxLifetime,

		Compression: &ch.Compression{
			Method: ch.CompressionLZ4,
		},
		Settings: ch.Settings{
			"max_execution_time":            p.config.MaxExecutionTime.Milliseconds(),
			"max_block_size":                p.config.MaxBlockSize,
			"min_insert_block_size_rows":    p.config.MinInsertBlockSizeRows,
			"min_insert_block_size_bytes":   p.config.MinInsertBlockSizeBytes,
			"async_insert":                  1,
			"wait_for_async_insert_timeout": 30,
			"async_insert_busy_timeout_ms":  1000,
		},
	}

	conn, err := ch.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	return conn, nil
}

// Get 从连接池获取连接
func (p *CHPool) Get(ctx context.Context) (chlib.Conn, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, fmt.Errorf("connection pool is closed")
	}
	p.mu.RUnlock()

	select {
	case conn := <-p.connections:
		// 检查连接是否仍然有效
		if err := conn.Ping(ctx); err != nil {
			// 连接无效，创建新连接
			conn.Close()
			newConn, err := p.createConnection()
			if err != nil {
				return nil, fmt.Errorf("failed to create replacement connection: %w", err)
			}
			p.mu.Lock()
			p.stats.ActiveConnections++
			p.stats.IdleConnections--
			p.mu.Unlock()
			return newConn, nil
		}

		p.mu.Lock()
		p.stats.ActiveConnections++
		p.stats.IdleConnections--
		p.mu.Unlock()
		return conn, nil

	case <-ctx.Done():
		return nil, ctx.Err()

	default:
		// 连接池为空，尝试创建新连接
		p.mu.Lock()
		if p.stats.TotalConnections < p.config.MaxConnections {
			p.mu.Unlock()
			conn, err := p.createConnection()
			if err != nil {
				return nil, fmt.Errorf("failed to create new connection: %w", err)
			}
			p.mu.Lock()
			p.stats.TotalConnections++
			p.stats.ActiveConnections++
			p.mu.Unlock()
			return conn, nil
		}
		p.mu.Unlock()

		// 等待可用连接
		select {
		case conn := <-p.connections:
			p.mu.Lock()
			p.stats.ActiveConnections++
			p.stats.IdleConnections--
			p.mu.Unlock()
			return conn, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Put 将连接返回到连接池
func (p *CHPool) Put(conn chlib.Conn) {
	if conn == nil {
		return
	}

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		conn.Close()
		return
	}
	p.mu.RUnlock()

	select {
	case p.connections <- conn:
		p.mu.Lock()
		p.stats.ActiveConnections--
		p.stats.IdleConnections++
		p.mu.Unlock()
	default:
		// 连接池已满，关闭连接
		conn.Close()
		p.mu.Lock()
		p.stats.ActiveConnections--
		p.stats.TotalConnections--
		p.mu.Unlock()
	}
}

// Query 执行查询（自动管理连接）
func (p *CHPool) Query(ctx context.Context, query string, args ...interface{}) (chlib.Rows, error) {
	conn, err := p.Get(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		p.Put(conn)
		p.mu.Lock()
		p.stats.FailedQueries++
		p.mu.Unlock()
		return nil, err
	}

	p.mu.Lock()
	p.stats.TotalQueries++
	p.stats.LastQueryTime = time.Now()
	p.mu.Unlock()

	// 创建一个包装的 Rows，在关闭时自动返回连接
	return &pooledRows{
		Rows: rows,
		conn: conn,
		pool: p,
	}, nil
}

// Exec 执行命令（自动管理连接）
func (p *CHPool) Exec(ctx context.Context, query string, args ...interface{}) error {
	conn, err := p.Get(ctx)
	if err != nil {
		return err
	}
	defer p.Put(conn)

	err = conn.Exec(ctx, query, args...)
	if err != nil {
		p.mu.Lock()
		p.stats.FailedQueries++
		p.mu.Unlock()
		return err
	}

	p.mu.Lock()
	p.stats.TotalQueries++
	p.stats.LastQueryTime = time.Now()
	p.mu.Unlock()
	return nil
}

// Ping 检查连接池健康状态
func (p *CHPool) Ping(ctx context.Context) error {
	conn, err := p.Get(ctx)
	if err != nil {
		return err
	}
	defer p.Put(conn)
	return conn.Ping(ctx)
}

// Stats 获取连接池统计信息
func (p *CHPool) Stats() CHPoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return *p.stats
}

// Close 关闭连接池
func (p *CHPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	close(p.connections)

	// 关闭所有连接
	for conn := range p.connections {
		conn.Close()
	}

	return nil
}

// cleanupRoutine 定期清理无效连接
func (p *CHPool) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.cleanupConnections()
	}
}

// cleanupConnections 清理无效连接
func (p *CHPool) cleanupConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	// 检查空闲连接
	validConnections := make([]chlib.Conn, 0, len(p.connections))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		select {
		case conn := <-p.connections:
			if err := conn.Ping(ctx); err != nil {
				// 连接无效，关闭它
				conn.Close()
				p.stats.TotalConnections--
				p.stats.IdleConnections--
			} else {
				validConnections = append(validConnections, conn)
			}
		default:
			goto done
		}
	}

done:
	// 将有效连接放回池中
	for _, conn := range validConnections {
		select {
		case p.connections <- conn:
		default:
			// 池已满，关闭连接
			conn.Close()
			p.stats.TotalConnections--
			p.stats.IdleConnections--
		}
	}

	// 确保最小连接数
	for p.stats.TotalConnections < p.config.MinConnections {
		conn, err := p.createConnection()
		if err != nil {
			log.Printf("Failed to create connection during cleanup: %v", err)
			break
		}
		select {
		case p.connections <- conn:
			p.stats.TotalConnections++
			p.stats.IdleConnections++
		default:
			conn.Close()
		}
	}
}

// pooledRows 包装的 Rows，自动管理连接
type pooledRows struct {
	chlib.Rows
	conn chlib.Conn
	pool *CHPool
}

// Close 关闭 Rows 并返回连接
func (pr *pooledRows) Close() error {
	err := pr.Rows.Close()
	pr.pool.Put(pr.conn)
	return err
}
