package main

import (
	"context"
	"log"
	"time"

	pb "github.com/bryanchen463/quota_data_service/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// QuotaServiceServer 实现 protobuf 中定义的 QuotaService 服务
type QuotaServiceServer struct {
	pb.UnimplementedQuotaServiceServer
	input chan<- Tick
}

// NewQuotaServiceServer 创建新的gRPC服务实例
func NewQuotaServiceServer(input chan<- Tick) *QuotaServiceServer {
	return &QuotaServiceServer{
		input: input,
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
			break
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
	if tick.MarketType != "spot" && tick.MarketType != "futures" {
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
