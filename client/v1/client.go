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
	defer conn.Close()

	client := pb.NewQuotaServiceClient(conn)

	response, err := client.GetLatestTick(ctx, &pb.GetLatestTickRequest{Symbol: symbol, Exchange: exchange, MarketType: marketType})
	if err != nil {
		return nil, err
	}
	return response.Tick, nil
}

func GetTicks(ctx context.Context, symbol string, exchange string, marketType string, startTimestamp int64, endTimestamp int64) ([]*pb.Tick, error) {
	conn, err := p.Get(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewQuotaServiceClient(conn)
	response, err := client.GetTicks(ctx, &pb.GetTicksRequest{Symbol: symbol, Exchange: exchange, MarketType: marketType, StartTime: startTimestamp, EndTime: endTimestamp})
	if err != nil {
		return nil, err
	}
	return response.Ticks, nil
}

func InsertTick(ctx context.Context, tick *pb.Tick) error {
	conn, err := p.Get(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

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
