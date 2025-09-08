package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	pb "github.com/bryanchen463/quota_data_service/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// QuotaServiceServer 实现 protobuf 中定义的 QuotaService 服务
type QuotaServiceServer struct {
	pb.UnimplementedQuotaServiceServer
	input chan<- Tick
	pool  *CHPool
}

// NewQuotaServiceServer 创建新的gRPC服务实例
func NewQuotaServiceServer(input chan<- Tick, pool *CHPool) *QuotaServiceServer {
	return &QuotaServiceServer{
		input: input,
		pool:  pool,
	}
}

// IngestTick 处理单个行情数据推送
func (s *QuotaServiceServer) IngestTick(ctx context.Context, req *pb.IngestTickRequest) (*pb.IngestTickResponse, error) {
	if req.Tick == nil {
		return nil, status.Error(codes.InvalidArgument, "tick data is required")
	}

	// 转换protobuf Tick到内部Tick结构
	tick := Tick{
		ReceiveTime: req.Tick.ReceiveTime,
		Symbol:      req.Tick.Symbol,
		Exchange:    req.Tick.Exchange,
		MarketType:  req.Tick.MarketType,
		BestBidPx:   req.Tick.BestBidPx,
		BestBidSz:   req.Tick.BestBidSz,
		BestAskPx:   req.Tick.BestAskPx,
		BestAskSz:   req.Tick.BestAskSz,
		BidsPx:      req.Tick.BidsPx,
		BidsSz:      req.Tick.BidsSz,
		AsksPx:      req.Tick.AsksPx,
		AsksSz:      req.Tick.AsksSz,
	}

	// 验证数据
	if err := s.validateTick(&tick); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tick data: %v", err)
	}

	// 发送到批处理器
	select {
	case s.input <- tick:
		return &pb.IngestTickResponse{
			Success: true,
			Message: "tick ingested successfully",
		}, nil
	default:
		// 队列已满，返回错误
		return &pb.IngestTickResponse{
			Success: false,
			Message: "queue is full, tick dropped",
		}, status.Error(codes.ResourceExhausted, "queue is full")
	}
}

// IngestTicks 处理批量行情数据推送
func (s *QuotaServiceServer) IngestTicks(ctx context.Context, req *pb.IngestTicksRequest) (*pb.IngestTicksResponse, error) {
	if len(req.Ticks) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ticks data is required")
	}

	if len(req.Ticks) > 1000 { // 限制批量大小
		return nil, status.Error(codes.InvalidArgument, "batch size too large, max 1000")
	}

	processedCount := 0
batchLoop:
	for _, pbTick := range req.Ticks {
		if pbTick == nil {
			continue
		}

		// 转换protobuf Tick到内部Tick结构
		tick := Tick{
			ReceiveTime: pbTick.ReceiveTime,
			Symbol:      pbTick.Symbol,
			Exchange:    pbTick.Exchange,
			MarketType:  pbTick.MarketType,
			BestBidPx:   pbTick.BestBidPx,
			BestBidSz:   pbTick.BestBidSz,
			BestAskPx:   pbTick.BestAskPx,
			BestAskSz:   pbTick.BestAskSz,
			BidsPx:      pbTick.BidsPx,
			BidsSz:      pbTick.BidsSz,
			AsksPx:      pbTick.AsksPx,
			AsksSz:      pbTick.AsksSz,
		}

		// 验证数据
		if err := s.validateTick(&tick); err != nil {
			log.Printf("invalid tick in batch: %v", err)
			continue
		}

		// 发送到批处理器
		select {
		case s.input <- tick:
			processedCount++
		default:
			// 队列已满，跳过剩余数据
			log.Printf("queue is full, dropping remaining ticks in batch")
			break batchLoop
		}
	}

	return &pb.IngestTicksResponse{
		Success:        processedCount > 0,
		Message:        "batch processing completed",
		ProcessedCount: int32(processedCount),
	}, nil
}

// HealthCheck 健康检查
func (s *QuotaServiceServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Healthy:   true,
		Message:   "service is healthy",
		Timestamp: time.Now().Unix(),
	}, nil
}

// validateTick 验证行情数据的有效性
func (s *QuotaServiceServer) validateTick(tick *Tick) error {
	if tick.Symbol == "" {
		return status.Error(codes.InvalidArgument, "symbol is required")
	}
	if tick.Exchange == "" {
		return status.Error(codes.InvalidArgument, "exchange is required")
	}
	if tick.MarketType == "" {
		return status.Error(codes.InvalidArgument, "market_type is required")
	}
	if tick.MarketType != "SP" && tick.MarketType != "UM" {
		return status.Error(codes.InvalidArgument, "market_type must be 'spot' or 'futures'")
	}
	if tick.ReceiveTime <= 0 {
		return status.Error(codes.InvalidArgument, "receive_time must be positive")
	}
	if tick.BestBidPx < 0 || tick.BestAskPx < 0 {
		return status.Error(codes.InvalidArgument, "prices must be non-negative")
	}
	if tick.BestBidSz < 0 || tick.BestAskSz < 0 {
		return status.Error(codes.InvalidArgument, "sizes must be non-negative")
	}

	// 验证数组长度一致性
	if len(tick.BidsPx) != len(tick.BidsSz) {
		return status.Error(codes.InvalidArgument, "bids_px and bids_sz arrays must have same length")
	}
	if len(tick.AsksPx) != len(tick.AsksSz) {
		return status.Error(codes.InvalidArgument, "asks_px and asks_sz arrays must have same length")
	}

	return nil
}

func (s *QuotaServiceServer) GetTicks(ctx context.Context, req *pb.GetTicksRequest) (*pb.GetTicksResponse, error) {
	// 构建查询条件
	whereConditions := []string{}
	args := []interface{}{}

	if req.Symbol != "" {
		if strings.Contains(req.Symbol, "*") {
			// 支持通配符查询
			pattern := strings.ReplaceAll(req.Symbol, "*", "%")
			whereConditions = append(whereConditions, "symbol LIKE ?")
			args = append(args, pattern)
		} else {
			whereConditions = append(whereConditions, "symbol = ?")
			args = append(args, req.Symbol)
		}
	}

	if req.Exchange != "" {
		whereConditions = append(whereConditions, "exchange = ?")
		args = append(args, req.Exchange)
	}

	if req.MarketType != "" {
		whereConditions = append(whereConditions, "market_type = ?")
		args = append(args, req.MarketType)
	}

	if req.StartTime > 0 {
		whereConditions = append(whereConditions, "receive_time >= ?")
		args = append(args, time.Unix(req.StartTime, 0))
	}

	if req.EndTime > 0 {
		whereConditions = append(whereConditions, "receive_time <= ?")
		args = append(args, time.Unix(req.EndTime, 0))
	}

	// 设置默认限制
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// 将 Interval 枚举转换为毫秒值
	intervalMs := intervalToMilliseconds(req.Interval)
	ticks, err := getTickers(ctx, whereConditions, args, int(limit), int(offset), intervalMs, s.pool)
	if err != nil {
		return &pb.GetTicksResponse{
			Success: false,
			Message: fmt.Sprintf("get ticks failed: %v", err),
		}, err
	}
	var pbTicks []*pb.Tick
	for _, tick := range ticks {
		pbTicks = append(pbTicks, convertTick(&tick, tick.ReceiveTime))
	}

	return &pb.GetTicksResponse{
		Success:    true,
		Message:    "query completed successfully",
		Ticks:      pbTicks,
		TotalCount: int32(len(ticks)),
	}, nil
}

func (s *QuotaServiceServer) GetLatestTick(ctx context.Context, req *pb.GetLatestTickRequest) (*pb.GetLatestTickResponse, error) {
	// 验证必填参数
	if req.Symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}

	// 构建查询条件
	whereConditions := []string{"symbol = ?"}
	args := []interface{}{req.Symbol}

	if req.Exchange != "" {
		whereConditions = append(whereConditions, "exchange = ?")
		args = append(args, req.Exchange)
	}

	if req.MarketType != "" {
		whereConditions = append(whereConditions, "market_type = ?")
		args = append(args, req.MarketType)
	}

	t, receiveTime, bidsPx, bidsSz, asksPx, asksSz, err := getTick(whereConditions, s.pool, nil, args, nil)
	if err != nil {
		return &pb.GetLatestTickResponse{
			Success: false,
			Message: fmt.Sprintf("get tick failed: %v", err),
		}, err
	}
	tick := convertTick(&t, float64(receiveTime.Unix())+float64(receiveTime.Nanosecond())/1e9)

	// 转换时间戳
	tick.ReceiveTime = float64(receiveTime.Unix()) + float64(receiveTime.Nanosecond())/1e9
	tick.BidsPx = bidsPx
	tick.BidsSz = bidsSz
	tick.AsksPx = asksPx
	tick.AsksSz = asksSz

	return &pb.GetLatestTickResponse{
		Success: true,
		Message: "latest tick retrieved successfully",
		Tick:    tick,
	}, nil
}

func convertTick(tick *Tick, receiveTime float64) *pb.Tick {
	return &pb.Tick{
		Symbol:      tick.Symbol,
		Exchange:    tick.Exchange,
		MarketType:  tick.MarketType,
		BestBidPx:   tick.BestBidPx,
		BestBidSz:   tick.BestBidSz,
		BestAskPx:   tick.BestAskPx,
		BestAskSz:   tick.BestAskSz,
		BidsPx:      tick.BidsPx,
		BidsSz:      tick.BidsSz,
		AsksPx:      tick.AsksPx,
		AsksSz:      tick.AsksSz,
		ReceiveTime: receiveTime,
	}
}

func (s *QuotaServiceServer) GetActiveSymbols(ctx context.Context, req *pb.GetActiveSymbolsRequest) (*pb.GetActiveSymbolsResponse, error) {
	whereConditions := []string{}
	args := []interface{}{}

	if req.Exchange != "" {
		whereConditions = append(whereConditions, "exchange = ?")
		args = append(args, req.Exchange)
	}

	if req.MarketType != "" {
		whereConditions = append(whereConditions, "market_type = ?")
		args = append(args, req.MarketType)
	}
	symbols, err := getActiveSymbols(ctx, whereConditions, s.pool, args)
	if err != nil {
		return &pb.GetActiveSymbolsResponse{
			Success: false,
			Message: fmt.Sprintf("get active symbols failed: %v", err),
		}, err
	}
	return &pb.GetActiveSymbolsResponse{
		Success: true,
		Message: "active symbols retrieved successfully",
		Symbols: symbols,
	}, nil
}
