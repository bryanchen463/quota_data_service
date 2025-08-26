package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	pb "github.com/bryanchen463/quota_data_service/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 连接到gRPC服务器
	conn, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewQuotaServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试健康检查
	log.Println("Testing health check...")
	healthResp, err := client.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		log.Fatalf("health check failed: %v", err)
	}
	log.Printf("Health check response: %v", healthResp)

	// 测试单个行情推送
	log.Println("Testing single tick ingestion...")
	tick := createSampleTick()
	resp, err := client.IngestTick(ctx, &pb.IngestTickRequest{Tick: tick})
	if err != nil {
		log.Fatalf("failed to ingest tick: %v", err)
	}
	log.Printf("Single tick response: %v", resp)

	// 测试批量行情推送
	log.Println("Testing batch tick ingestion...")
	ticks := createSampleTicks(10)
	batchResp, err := client.IngestTicks(ctx, &pb.IngestTicksRequest{Ticks: ticks})
	if err != nil {
		log.Fatalf("failed to ingest batch ticks: %v", err)
	}
	log.Printf("Batch ticks response: %v", batchResp)

	// 持续推送数据（模拟实时行情）
	log.Println("Starting continuous data ingestion...")
	ticker := time.NewTicker(100 * time.Millisecond) // 每100ms推送一次
	defer ticker.Stop()

	count := 0
	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping...")
			return
		case <-ticker.C:
			tick := createRandomTick()
			_, err := client.IngestTick(ctx, &pb.IngestTickRequest{Tick: tick})
			if err != nil {
				log.Printf("Failed to ingest tick: %v", err)
				continue
			}
			count++
			if count%100 == 0 {
				log.Printf("Ingested %d ticks", count)
			}
		}
	}
}

// createSampleTick 创建示例行情数据
func createSampleTick() *pb.Tick {
	return &pb.Tick{
		ReceiveTime: float64(time.Now().Unix()),
		Symbol:      "btcusdt",
		Exchange:    "binance",
		MarketType:  "spot",
		BestBidPx:   60000.0,
		BestBidSz:   0.5,
		BestAskPx:   60001.0,
		BestAskSz:   0.4,
		BidsPx:      []float64{60000, 59999, 59998},
		BidsSz:      []float64{1, 2, 3},
		AsksPx:      []float64{60001, 60002, 60003},
		AsksSz:      []float64{1.5, 2.5, 3.5},
	}
}

// createSampleTicks 创建多个示例行情数据
func createSampleTicks(count int) []*pb.Tick {
	ticks := make([]*pb.Tick, count)
	symbols := []string{"btcusdt", "ethusdt", "bnbusdt", "adausdt", "dogeusdt"}
	exchanges := []string{"binance", "okx", "bybit", "gate", "kucoin"}

	for i := 0; i < count; i++ {
		ticks[i] = &pb.Tick{
			ReceiveTime: float64(time.Now().Unix()) + float64(i),
			Symbol:      symbols[i%len(symbols)],
			Exchange:    exchanges[i%len(exchanges)],
			MarketType:  "spot",
			BestBidPx:   50000 + float64(i*100),
			BestBidSz:   0.1 + float64(i)*0.1,
			BestAskPx:   50001 + float64(i*100),
			BestAskSz:   0.1 + float64(i)*0.1,
			BidsPx:      []float64{50000 + float64(i*100), 49999 + float64(i*100)},
			BidsSz:      []float64{1, 2},
			AsksPx:      []float64{50001 + float64(i*100), 50002 + float64(i*100)},
			AsksSz:      []float64{1.5, 2.5},
		}
	}
	return ticks
}

// createRandomTick 创建随机行情数据
func createRandomTick() *pb.Tick {
	basePrice := 50000 + rand.Float64()*20000 // 50000-70000
	spread := 1 + rand.Float64()*10           // 1-11

	return &pb.Tick{
		ReceiveTime: float64(time.Now().Unix()),
		Symbol:      "btcusdt",
		Exchange:    "binance",
		MarketType:  "spot",
		BestBidPx:   basePrice,
		BestBidSz:   0.1 + rand.Float64()*2,
		BestAskPx:   basePrice + spread,
		BestAskSz:   0.1 + rand.Float64()*2,
		BidsPx:      []float64{basePrice, basePrice - 1, basePrice - 2},
		BidsSz:      []float64{1 + rand.Float64()*5, 2 + rand.Float64()*5, 3 + rand.Float64()*5},
		AsksPx:      []float64{basePrice + spread, basePrice + spread + 1, basePrice + spread + 2},
		AsksSz:      []float64{1 + rand.Float64()*5, 2 + rand.Float64()*5, 3 + rand.Float64()*5},
	}
}
