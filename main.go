// main.go
// ─────────────────────────────────────────────────────────────────────────────
// 功能
// - 接收已有采集服务推送的行情（HTTP POST /ingest）
// - 数据模型：receive_time + symbol + exchange + market_type(现货/合约) + BBO + depth
// - 10ms 批量写入 ClickHouse（或达到最大批量阈值）
// - 自动建表（DateTime64(6) 微秒精度）
//
// 使用
//  1. 修改下方 CONFIG 默认值或通过环境变量覆盖
//  2. go mod tidy && go run .
//  3. 推送数据：
//     curl -X POST http://127.0.0.1:8080/ingest -H 'Content-Type: application/json' \
//     -d '{"receive_time": 1724131200.123456, "symbol":"btcusdt","exchange":"binance","market_type":"spot","best_bid_px":60000,"best_bid_sz":0.5,"best_ask_px":60001,"best_ask_sz":0.4,"bids_px":[60000,59999],"bids_sz":[1,2],"asks_px":[60001,60002],"asks_sz":[1.5,2.5]}'
//     # 亦支持 NDJSON：多行 JSONEachRow
//
// 依赖
//
//	go get github.com/ClickHouse/clickhouse-go/v2
//
// 建表 SQL（程序启动会自动执行）：
//
//	CREATE TABLE IF NOT EXISTS crypto_market.raw_ticks (
//	  receive_time   DateTime64(6) CODEC(DoubleDelta, ZSTD(3)),
//	  symbol         LowCardinality(String),
//	  exchange       LowCardinality(String),
//	  market_type    LowCardinality(String),    -- spot | futures
//	  best_bid_px    Float64,
//	  best_bid_sz    Float64,
//	  best_ask_px    Float64,
//	  best_ask_sz    Float64,
//	  bids_px        Array(Float64),
//	  bids_sz        Array(Float64),
//	  asks_px        Array(Float64),
//	  asks_sz        Array(Float64)
//	) ENGINE = MergeTree
//	PARTITION BY toDate(receive_time)
//	ORDER BY (symbol, receive_time)
//	SETTINGS index_granularity = 8192;
//
// ─────────────────────────────────────────────────────────────────────────────
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	pb "github.com/bryanchen463/quota_data_service/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var ErrNoData = errors.New("no data found for the specified symbol")

//go:embed docs/openapi.yaml
var openapiYAML []byte

// Tick 与 ClickHouse 表字段一一对应
// receive_time 使用秒（float64）传入，写入前转换为 time.Time（微秒精度）
type Tick struct {
	ReceiveTime float64   `json:"receive_time"`
	Symbol      string    `json:"symbol"`
	Exchange    string    `json:"exchange"`
	MarketType  string    `json:"market_type"` // spot|futures
	BestBidPx   float64   `json:"best_bid_px"`
	BestBidSz   float64   `json:"best_bid_sz"`
	BestAskPx   float64   `json:"best_ask_px"`
	BestAskSz   float64   `json:"best_ask_sz"`
	BidsPx      []float64 `json:"bids_px"`
	BidsSz      []float64 `json:"bids_sz"`
	AsksPx      []float64 `json:"asks_px"`
	AsksSz      []float64 `json:"asks_sz"`
}

// Config 可通过环境变量覆盖
var CONFIG = struct {
	HTTPAddr      string
	GRPCAddr      string
	CHAddr        string
	CHUser        string
	CHPassword    string
	CHDatabase    string
	CHTable       string
	FlushInterval time.Duration // 冲刷间隔，默认 10ms
	MaxBatch      int           // 最大批量
	MaxQueue      int           // 队列容量
	InsertTimeout time.Duration
}{
	HTTPAddr:      getenv("INGEST_ADDR", ":8080"),
	GRPCAddr:      getenv("GRPC_ADDR", ":9090"),
	CHAddr:        getenv("CH_ADDR", "127.0.0.1:9000"),
	CHUser:        os.Getenv("CH_USER"),
	CHPassword:    os.Getenv("CH_PASSWORD"),
	CHDatabase:    getenv("CH_DATABASE", "crypto_market"),
	CHTable:       getenv("CH_TABLE", "raw_ticks"),
	FlushInterval: getenvDuration("FLUSH_MS", 10*time.Millisecond),
	MaxBatch:      getenvInt("MAX_BATCH", 800),
	MaxQueue:      getenvInt("MAX_QUEUE", 20000),
	InsertTimeout: getenvDuration("INSERT_TIMEOUT_MS", 30000*time.Millisecond), // 增加到30秒
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var x int
		fmt.Sscanf(v, "%d", &x)
		if x > 0 {
			return x
		}
	}
	return def
}
func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		var x int
		fmt.Sscanf(v, "%d", &x)
		if x > 0 {
			return time.Duration(x) * time.Millisecond
		}
	}
	return def
}

// ClickHouse 客户端与批量写入器
type CHWriter struct {
	pool  *CHPool
	db    string
	table string
	stmt  string
	mu    sync.Mutex
}

// Ping 检查 ClickHouse 连接健康状态
func (w *CHWriter) Ping(ctx context.Context) error {
	return w.pool.Ping(ctx)
}

func NewCHWriter(ctx context.Context, addr, user, pass, db, table string) (*CHWriter, error) {
	// 创建连接池配置
	poolConfig := &CHPoolConfig{
		Addr:     addr,
		User:     user,
		Password: pass,
		Database: db,
		Table:    table,

		// 连接池配置
		MaxConnections: 10,
		MinConnections: 2,
		MaxIdleTime:    5 * time.Minute,
		MaxLifetime:    30 * time.Minute,

		// 连接选项
		DialTimeout:             10 * time.Second,
		MaxOpenConns:            10,
		MaxIdleConns:            5,
		ConnMaxLifetime:         30 * time.Minute,
		ConnMaxIdleTime:         5 * time.Minute,
		Compression:             true,
		MaxExecutionTime:        60 * time.Second,
		MaxBlockSize:            100000,
		MinInsertBlockSizeRows:  1000,
		MinInsertBlockSizeBytes: 268435456,
	}

	// 创建连接池
	pool, err := NewCHPool(poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clickhouse connection pool: %w", err)
	}

	// 测试连接池
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping clickhouse pool: %w", err)
	}

	w := &CHWriter{pool: pool, db: db, table: table}
	if err := w.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	w.stmt = fmt.Sprintf("INSERT INTO %s.%s (receive_time,symbol,exchange,market_type,best_bid_px,best_bid_sz,best_ask_px,best_ask_sz,bids_px,bids_sz,asks_px,asks_sz) VALUES", db, table)
	return w, nil
}

func (w *CHWriter) ensureSchema(ctx context.Context) error {
	sql := `CREATE TABLE IF NOT EXISTS %s.%s (
		receive_time   DateTime64(6) CODEC(DoubleDelta, ZSTD(3)),
		symbol         LowCardinality(String),
		exchange       LowCardinality(String),
		market_type    LowCardinality(String),
		best_bid_px    Float64,
		best_bid_sz    Float64,
		best_ask_px    Float64,
		best_ask_sz    Float64,
		bids_px        Array(Float64),
		bids_sz        Array(Float64),
		asks_px        Array(Float64),
		asks_sz        Array(Float64)
	) ENGINE = MergeTree
	PARTITION BY toDate(receive_time)
	ORDER BY (symbol, receive_time)
	SETTINGS index_granularity = 8192;`
	q := fmt.Sprintf(sql, w.db, w.table)
	return w.pool.Exec(ctx, q)
}

func (w *CHWriter) InsertBatch(ctx context.Context, tickers []Tick) error {
	if len(tickers) == 0 {
		return nil
	}

	// 重试插入逻辑
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ { // 最多重试3次
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 从连接池获取连接
		conn, err := w.pool.Get(ctx)
		if err != nil {
			lastErr = fmt.Errorf("failed to get connection from pool: %v", err)
			log.Printf("ClickHouse get connection attempt %d failed: %v", attempt, lastErr)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return lastErr
		}

		// 准备批量插入
		batch, err := conn.PrepareBatch(ctx, w.stmt)
		if err != nil {
			w.pool.Put(conn)
			lastErr = fmt.Errorf("prepare batch failed: %v", err)
			log.Printf("ClickHouse prepare batch attempt %d failed: %v", attempt, lastErr)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return lastErr
		}

		// 添加数据到批次
		for _, t := range tickers {
			// 转换时间戳：float64 -> time.Time
			receiveTime := time.Unix(int64(t.ReceiveTime), int64((t.ReceiveTime-float64(int64(t.ReceiveTime)))*1e9))
			batch.Append(receiveTime, t.Symbol, t.Exchange, t.MarketType, t.BestBidPx, t.BestBidSz, t.BestAskPx, t.BestAskSz, t.BidsPx, t.BidsSz, t.AsksPx, t.AsksSz)
		}

		// 执行插入
		err = batch.Send()
		w.pool.Put(conn) // 无论成功失败都要返回连接

		if err == nil {
			log.Printf("successfully inserted %d ticks to ClickHouse", len(tickers))
			return nil
		}

		lastErr = fmt.Errorf("batch send failed: %v", err)
		log.Printf("ClickHouse insert attempt %d failed: %v", attempt, lastErr)

		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return fmt.Errorf("all insert attempts failed, last error: %v", lastErr)
}

// Batcher：10ms 定时或达到上限就 Flush
type Batcher struct {
	in            chan Tick
	buf           []Tick
	flushEvery    time.Duration
	maxBatch      int
	writer        *CHWriter
	insertTimeout time.Duration
}

func NewBatcher(writer *CHWriter, flushEvery time.Duration, maxBatch int, insertTimeout time.Duration, queue int) *Batcher {
	log.Printf("NewBatcher: flushEvery=%s, maxBatch=%d, insertTimeout=%s, queue=%d", flushEvery, maxBatch, insertTimeout, queue)
	return &Batcher{in: make(chan Tick, queue), flushEvery: flushEvery, maxBatch: maxBatch, writer: writer, insertTimeout: insertTimeout}
}

func (b *Batcher) Input() chan<- Tick { return b.in }

func (b *Batcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(b.flushEvery)
	healthTicker := time.NewTicker(30 * time.Second) // 每30秒检查一次连接健康状态
	defer ticker.Stop()
	defer healthTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.flush(ctx)
			return ctx.Err()
		case t := <-b.in:
			b.buf = append(b.buf, t)
			if len(b.buf) >= b.maxBatch {
				b.flush(ctx)
			}
		case <-ticker.C:
			b.flush(ctx)
		case <-healthTicker.C:
			// 检查 ClickHouse 连接健康状态
			if err := b.writer.Ping(ctx); err != nil {
				log.Printf("ClickHouse health check failed: %v", err)
			} else {
				log.Printf("ClickHouse connection healthy")
			}
		}
	}
}

func (b *Batcher) flush(ctx context.Context) {
	if len(b.buf) == 0 {
		return
	}

	batch := make([]Tick, len(b.buf))
	copy(batch, b.buf)
	b.buf = b.buf[:0]

	log.Printf("flushing %d ticks to ClickHouse...", len(batch))

	c, cancel := context.WithTimeout(ctx, b.insertTimeout)
	defer cancel()

	start := time.Now()
	if err := b.writer.InsertBatch(c, batch); err != nil {
		log.Printf("flush error: %v (dropped=%d, duration=%v)", err, len(batch), time.Since(start))

		// 如果是因为超时导致的错误，尝试将数据放回缓冲区
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("insert timeout, attempting to requeue %d ticks", len(batch))
			// 注意：这里可能会导致数据重复，但在超时情况下是合理的
			for _, tick := range batch {
				select {
				case b.in <- tick:
					// 成功放回
				default:
					log.Printf("failed to requeue tick, dropping: %+v", tick)
				}
			}
		}
	} else {
		log.Printf("flush successful: %d ticks in %v", len(batch), time.Since(start))
	}
}

// HTTP Ingest Handler：支持单 JSON 对象、JSON 数组、以及 NDJSON（每行一个 JSON）
func makeIngestHandler(input chan<- Tick) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "json") {
			// 尝试区分 NDJSON 与普通 JSON
			scanner := bufio.NewScanner(r.Body)
			scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // up to 10MB/line
			peeked := false
			var firstLine string
			if scanner.Scan() {
				firstLine = scanner.Text()
				peeked = true
			}
			err := scanner.Err()
			if err != nil && !errors.Is(err, bufio.ErrFinalToken) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("scan error"))
				return
			}
			if peeked && (strings.HasPrefix(strings.TrimSpace(firstLine), "{") || strings.HasPrefix(strings.TrimSpace(firstLine), "[")) {
				// 如果是数组或对象，读取整个 body
				// 重新构造 body：firstLine + 剩余
				rest := firstLine
				for scanner.Scan() {
					rest += "\n" + scanner.Text()
				}
				var ticks []Tick
				trim := strings.TrimSpace(rest)
				if strings.HasPrefix(trim, "[") { // JSON 数组
					if err := json.Unmarshal([]byte(trim), &ticks); err != nil {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte("invalid json array"))
						return
					}
				} else { // 单对象
					var t Tick
					if err := json.Unmarshal([]byte(trim), &t); err != nil {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte("invalid json object"))
						return
					}
					ticks = []Tick{t}
				}
				for _, t := range ticks {
					select {
					case input <- t:
					default:
					}
				}
				w.WriteHeader(http.StatusAccepted)
				return
			}
			// NDJSON：逐行 JSONEachRow
			if peeked {
				var t Tick
				if err := json.Unmarshal([]byte(firstLine), &t); err == nil {
					select {
					case input <- t:
					default:
					}
				}
				for scanner.Scan() {
					line := scanner.Text()
					if strings.TrimSpace(line) == "" {
						continue
					}
					var tt Tick
					if err := json.Unmarshal([]byte(line), &tt); err == nil {
						select {
						case input <- tt:
						default:
						}
					}
				}
				w.WriteHeader(http.StatusAccepted)
				return
			}
		}
		w.WriteHeader(http.StatusUnsupportedMediaType)
	}
}

func main() {
	log.Printf("starting ingest on HTTP:%s, gRPC:%s, CH=%s db=%s table=%s flush=%s maxBatch=%d",
		CONFIG.HTTPAddr, CONFIG.GRPCAddr, CONFIG.CHAddr, CONFIG.CHDatabase, CONFIG.CHTable, CONFIG.FlushInterval, CONFIG.MaxBatch)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	writer, err := NewCHWriter(ctx, CONFIG.CHAddr, CONFIG.CHUser, CONFIG.CHPassword, CONFIG.CHDatabase, CONFIG.CHTable)
	if err != nil {
		log.Fatalf("clickhouse init: %v", err)
	}

	batcher := NewBatcher(writer, CONFIG.FlushInterval, CONFIG.MaxBatch, CONFIG.InsertTimeout, CONFIG.MaxQueue)
	go func() {
		if err := batcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("batcher exit: %v", err)
		}
	}()

	// 启动 HTTP 服务器
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) })
	http.HandleFunc("/ingest", makeIngestHandler(batcher.Input()))
	http.HandleFunc("/ticks", makeGetTicksHandler(writer.pool))
	http.HandleFunc("/latest", makeGetLatestTickHandler(writer.pool))

	// 提供 Swagger UI 静态页面与资源
	fs := http.FileServer(http.Dir("docs/swagger"))
	http.Handle("/swagger/", http.StripPrefix("/swagger/", fs))
	// 提供 OpenAPI 文档（嵌入二进制）
	http.HandleFunc("/swagger.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(openapiYAML)
	})

	httpSrv := &http.Server{Addr: CONFIG.HTTPAddr, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server error: %v", err)
		}
	}()

	// 启动 gRPC 服务器
	grpcSrv := grpc.NewServer()
	quotaService := NewQuotaServiceServer(batcher.Input(), writer.pool)
	pb.RegisterQuotaServiceServer(grpcSrv, quotaService)
	reflection.Register(grpcSrv) // 启用反射，方便调试

	go func() {
		listener, err := net.Listen("tcp", CONFIG.GRPCAddr)
		if err != nil {
			log.Fatalf("failed to listen gRPC: %v", err)
		}
		log.Printf("gRPC server listening on %s", CONFIG.GRPCAddr)
		if err := grpcSrv.Serve(listener); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down servers...")

	// 优雅关闭 HTTP 服务器
	httpShutdownCtx, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	_ = httpSrv.Shutdown(httpShutdownCtx)

	// 优雅关闭 gRPC 服务器
	// grpcShutdownCtx, cancel3 := context.WithTimeout(context.Background(), 2*time.Second)
	// defer cancel3()
	grpcSrv.GracefulStop()

	log.Printf("stopped")
}

// HTTP 查询接口处理器

// makeGetTicksHandler 创建获取行情数据的 HTTP 处理器
func makeGetTicksHandler(pool *CHPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// 解析查询参数
		symbol := r.URL.Query().Get("symbol")
		exchange := r.URL.Query().Get("exchange")
		marketType := r.URL.Query().Get("market_type")
		intervalStr := r.URL.Query().Get("interval")
		interval := int64(0) // 默认无间隔
		if intervalStr != "" {
			if i, err := strconv.ParseInt(intervalStr, 10, 64); err == nil {
				interval = i
			}
		}

		var startTime, endTime int64
		if st := r.URL.Query().Get("start_time"); st != "" {
			if t, err := strconv.ParseInt(st, 10, 64); err == nil {
				startTime = t
			}
		}
		if et := r.URL.Query().Get("end_time"); et != "" {
			if t, err := strconv.ParseInt(et, 10, 64); err == nil {
				endTime = t
			}
		}

		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
				limit = n
			}
		}

		offset := 0
		if o := r.URL.Query().Get("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n >= 0 {
				offset = n
			}
		}

		// 构建查询条件
		whereConditions := []string{}
		args := []interface{}{}

		if symbol != "" {
			if strings.Contains(symbol, "*") {
				pattern := strings.ReplaceAll(symbol, "*", "%")
				whereConditions = append(whereConditions, "symbol LIKE ?")
				args = append(args, pattern)
			} else {
				whereConditions = append(whereConditions, "symbol = ?")
				args = append(args, symbol)
			}
		}

		if exchange != "" {
			whereConditions = append(whereConditions, "exchange = ?")
			args = append(args, exchange)
		}

		if marketType != "" {
			whereConditions = append(whereConditions, "market_type = ?")
			args = append(args, marketType)
		}

		if startTime > 0 {
			whereConditions = append(whereConditions, "receive_time >= ?")
			args = append(args, time.Unix(startTime, 0))
		}

		if endTime > 0 {
			whereConditions = append(whereConditions, "receive_time <= ?")
			args = append(args, time.Unix(endTime, 0))
		}

		// 构建查询SQL
		ticks, err := getTickers(r.Context(), whereConditions, args, limit, offset, interval, pool)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "get tickers failed: %v"}`, err)))
			return
		}

		// 返回 JSON 响应
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"success":     true,
			"message":     "query completed successfully",
			"ticks":       ticks,
			"total_count": len(ticks),
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("GetTicks HTTP encode error: %v", err)
		}
	}
}

func getTickers(ctx context.Context, whereConditions []string, args []interface{}, limit int, offset int, interval int64, pool *CHPool) ([]Tick, error) {
	var query string

	// 如果指定了时间间隔，使用采样查询
	if interval > 0 {
		// 将时间戳转换为秒级间隔进行采样
		intervalSeconds := interval // 将毫秒转换为秒
		query = fmt.Sprintf(`
			SELECT 
				toStartOfInterval(receive_time, INTERVAL %d millisecond) as receive_time,
				symbol,
				exchange,
				market_type,
				argMax(best_bid_px, receive_time) as best_bid_px,
				argMax(best_bid_sz, receive_time) as best_bid_sz,
				argMax(best_ask_px, receive_time) as best_ask_px,
				argMax(best_ask_sz, receive_time) as best_ask_sz,
				argMax(bids_px, receive_time) as bids_px,
				argMax(bids_sz, receive_time) as bids_sz,
				argMax(asks_px, receive_time) as asks_px,
				argMax(asks_sz, receive_time) as asks_sz
			FROM %s.%s
		`, intervalSeconds, CONFIG.CHDatabase, CONFIG.CHTable)
	} else {
		// 原始查询，返回所有数据点
		query = fmt.Sprintf(`
			SELECT 
				receive_time,
				symbol,
				exchange,
				market_type,
				best_bid_px,
				best_bid_sz,
				best_ask_px,
				best_ask_sz,
				bids_px,
				bids_sz,
				asks_px,
				asks_sz
			FROM %s.%s
		`, CONFIG.CHDatabase, CONFIG.CHTable)
	}

	if len(whereConditions) > 0 {
		query += " WHERE " + strings.Join(whereConditions, " AND ")
	}

	// 对于采样查询，需要按时间间隔分组
	if interval > 0 {
		query += " GROUP BY receive_time, symbol, exchange, market_type"
	}

	query += " ORDER BY receive_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	log.Printf("query: %s", query)
	log.Printf("args: %v", args)
	// 执行查询
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %v", err)
	}
	defer rows.Close()

	// 解析结果
	ticks := make([]Tick, 0)
	for rows.Next() {
		var tick Tick
		var receiveTime time.Time
		var bidsPx, bidsSz, asksPx, asksSz []float64

		err := rows.Scan(
			&receiveTime,
			&tick.Symbol,
			&tick.Exchange,
			&tick.MarketType,
			&tick.BestBidPx,
			&tick.BestBidSz,
			&tick.BestAskPx,
			&tick.BestAskSz,
			&bidsPx,
			&bidsSz,
			&asksPx,
			&asksSz,
		)
		if err != nil {
			return nil, fmt.Errorf("scan failed: %v", err)
		}

		// 转换时间戳
		tick.ReceiveTime = float64(receiveTime.Unix()) + float64(receiveTime.Nanosecond())/1e9
		tick.BidsPx = bidsPx
		tick.BidsSz = bidsSz
		tick.AsksPx = asksPx
		tick.AsksSz = asksSz

		ticks = append(ticks, tick)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %v", err)
	}
	return ticks, nil
}

// intervalToMilliseconds 将 Interval 枚举转换为毫秒值
func intervalToMilliseconds(interval pb.Interval) int64 {
	switch interval {
	case pb.Interval_INTERVAL_1MS:
		return 1
	case pb.Interval_INTERVAL_5MS:
		return 5
	case pb.Interval_INTERVAL_10MS:
		return 10
	case pb.Interval_INTERVAL_30MS:
		return 30
	case pb.Interval_INTERVAL_100MS:
		return 100
	case pb.Interval_INTERVAL_1S:
		return 1000
	case pb.Interval_INTERVAL_5S:
		return 5000
	case pb.Interval_INTERVAL_10S:
		return 10000
	case pb.Interval_INTERVAL_30S:
		return 30000
	case pb.Interval_INTERVAL_1M:
		return 60000
	case pb.Interval_INTERVAL_5M:
		return 300000
	case pb.Interval_INTERVAL_15M:
		return 900000
	case pb.Interval_INTERVAL_30M:
		return 1800000
	case pb.Interval_INTERVAL_1H:
		return 3600000
	case pb.Interval_INTERVAL_4H:
		return 14400000
	case pb.Interval_INTERVAL_1D:
		return 86400000
	default:
		return 0 // 无间隔，返回原始数据
	}
}

// makeGetLatestTickHandler 创建获取最新行情的 HTTP 处理器
func makeGetLatestTickHandler(pool *CHPool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// 解析查询参数
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "symbol parameter is required"}`))
			return
		}

		exchange := r.URL.Query().Get("exchange")
		marketType := r.URL.Query().Get("market_type")

		// 构建查询条件
		whereConditions := []string{"symbol = ?"}
		args := []interface{}{symbol}

		if exchange != "" {
			whereConditions = append(whereConditions, "exchange = ?")
			args = append(args, exchange)
		}

		if marketType != "" {
			whereConditions = append(whereConditions, "market_type = ?")
			args = append(args, marketType)
		}

		// 构建查询SQL - 获取最新的一条记录
		tick, receiveTime, bidsPx, bidsSz, asksPx, asksSz, err := getTick(whereConditions, pool, r, args, w)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "get tick failed: %v"}`, err)))
			return
		}

		// 转换时间戳
		tick.ReceiveTime = float64(receiveTime.Unix()) + float64(receiveTime.Nanosecond())/1e9
		tick.BidsPx = bidsPx
		tick.BidsSz = bidsSz
		tick.AsksPx = asksPx
		tick.AsksSz = asksSz

		// 返回 JSON 响应
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"success": true,
			"message": "latest tick retrieved successfully",
			"tick":    tick,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("GetLatestTick HTTP encode error: %v", err)
		}
	}
}

func getTick(whereConditions []string, pool *CHPool, r *http.Request, args []interface{}, w http.ResponseWriter) (Tick, time.Time, []float64, []float64, []float64, []float64, error) {
	query := fmt.Sprintf(`
			SELECT 
				receive_time,
				symbol,
				exchange,
				market_type,
				best_bid_px,
				best_bid_sz,
				best_ask_px,
				best_ask_sz,
				bids_px,
				bids_sz,
				asks_px,
				asks_sz
			FROM %s.%s
			WHERE %s
			ORDER BY receive_time DESC
			LIMIT 1
		`, CONFIG.CHDatabase, CONFIG.CHTable, strings.Join(whereConditions, " AND "))

	// 执行查询
	rows, err := pool.Query(r.Context(), query, args...)
	if err != nil {
		log.Printf("GetLatestTick HTTP query error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error": "query failed: %v"}`, err)))
		return Tick{}, time.Time{}, nil, nil, nil, nil, err
	}
	defer rows.Close()

	// 解析结果
	if !rows.Next() {
		return Tick{}, time.Time{}, nil, nil, nil, nil, ErrNoData
	}

	var tick Tick
	var receiveTime time.Time
	var bidsPx, bidsSz, asksPx, asksSz []float64

	err = rows.Scan(
		&receiveTime,
		&tick.Symbol,
		&tick.Exchange,
		&tick.MarketType,
		&tick.BestBidPx,
		&tick.BestBidSz,
		&tick.BestAskPx,
		&tick.BestAskSz,
		&bidsPx,
		&bidsSz,
		&asksPx,
		&asksSz,
	)
	if err != nil {
		return Tick{}, time.Time{}, nil, nil, nil, nil, fmt.Errorf("scan failed: %v", err)
	}
	return tick, receiveTime, bidsPx, bidsSz, asksPx, asksSz, nil
}
