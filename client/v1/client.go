package client

import (
	"context"
	"errors"
	"time"

	pb "github.com/bryanchen463/quota_data_service/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var p *Pool

func InitPool(ctx context.Context, target string, init, cap int, idleTimeout time.Duration, maxLifetime time.Duration) error {
	pool, err := New(target, init, cap, idleTimeout, maxLifetime, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	p = pool
	return nil
}

func GetLatestTick(ctx context.Context, symbol string, exchange string, marketType string) (*pb.Tick, error) {
	conn, err := p.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer p.Put(conn)

	client := pb.NewQuotaServiceClient(conn)

	response, err := client.GetLatestTick(ctx, &pb.GetLatestTickRequest{Symbol: symbol, Exchange: exchange, MarketType: marketType})
	if err != nil {
		return nil, err
	}
	return response.Tick, nil
}

func GetTicks(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	return GetTicksWithInterval(ctx, symbol, exchange, marketType, startTimestamp, endTimestamp, pb.Interval_INTERVAL_1MS, 100, 0)
}

func GetTicksWithInterval(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64, interval pb.Interval, limit int32, offset int32) ([]*pb.Tick, error) {
	conn, err := p.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer p.Put(conn)

	client := pb.NewQuotaServiceClient(conn)
	response, err := client.GetTicks(ctx, &pb.GetTicksRequest{
		Symbol:     symbol,
		Exchange:   exchange,
		MarketType: marketType,
		StartTime:  startTimestamp,
		EndTime:    endTimestamp,
		Interval:   interval,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		return nil, err
	}
	return response.Ticks, nil
}

// 便捷的采样函数
func GetTicks1Second(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	return GetTicksWithInterval(ctx, symbol, exchange, marketType, startTimestamp, endTimestamp, pb.Interval_INTERVAL_1S, 100, 0)
}

func GetTicks5Second(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	return GetTicksWithInterval(ctx, symbol, exchange, marketType, startTimestamp, endTimestamp, pb.Interval_INTERVAL_5S, 100, 0)
}

func GetTicks1Minute(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	return GetTicksWithInterval(ctx, symbol, exchange, marketType, startTimestamp, endTimestamp, pb.Interval_INTERVAL_1M, 100, 0)
}

func GetTicks5Minute(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	return GetTicksWithInterval(ctx, symbol, exchange, marketType, startTimestamp, endTimestamp, pb.Interval_INTERVAL_5M, 100, 0)
}

func GetTicks1Hour(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	return GetTicksWithInterval(ctx, symbol, exchange, marketType, startTimestamp, endTimestamp, pb.Interval_INTERVAL_1H, 100, 0)
}

func GetTicks1Day(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	return GetTicksWithInterval(ctx, symbol, exchange, marketType, startTimestamp, endTimestamp, pb.Interval_INTERVAL_1D, 100, 0)
}

func InsertTick(ctx context.Context, tick *pb.Tick) error {
	conn, err := p.Get(ctx)
	if err != nil {
		return err
	}
	defer p.Put(conn)

	client := pb.NewQuotaServiceClient(conn)
	response, err := client.IngestTick(ctx, &pb.IngestTickRequest{Tick: tick})
	if err != nil {
		return err
	}
	if !response.Success {
		return errors.New(response.Message)
	}
	return nil
}

func InsertTicks(ctx context.Context, ticks []*pb.Tick) error {
	conn, err := p.Get(ctx)
	if err != nil {
		return err
	}
	defer p.Put(conn)

	client := pb.NewQuotaServiceClient(conn)
	response, err := client.IngestTicks(ctx, &pb.IngestTicksRequest{Ticks: ticks})
	if err != nil {
		return err
	}
	if !response.Success {
		return errors.New(response.Message)
	}
	return nil
}
