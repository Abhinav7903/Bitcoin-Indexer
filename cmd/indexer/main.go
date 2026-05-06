package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Abhinav7903/bitcoin-indexer/internal/db"
	"github.com/Abhinav7903/bitcoin-indexer/internal/pipeline"
	"github.com/Abhinav7903/bitcoin-indexer/pkg/rpc"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	rpcURLStr := os.Getenv("RPC_URL")
	if rpcURLStr == "" {
		log.Fatal("RPC_URL environment variable is required")
	}

	u, err := url.Parse(rpcURLStr)
	if err != nil {
		log.Fatalf("Invalid RPC_URL: %v", err)
	}

	rpcPass, _ := u.User.Password()
	rpcUser := u.User.Username()

	rpcEndpoint := fmt.Sprintf(
		"%s://%s%s",
		u.Scheme,
		u.Host,
		u.Path,
	)

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Unable to parse DATABASE_URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	rpcClient := rpc.NewClient(
		rpcEndpoint,
		rpcUser,
		rpcPass,
	)

	dbWriter := db.NewWriter(pool)

	workers := intEnv("WORKERS", 10)
	batchSize := intEnv("BATCH_SIZE", 100)

	lastHeight, err := dbWriter.GetLastHeight(ctx)
	if err != nil {
		log.Fatalf("Failed to get last height: %v", err)
	}

	startHeight := lastHeight + 1

	if s, err := strconv.Atoi(os.Getenv("START_HEIGHT")); err == nil {
		startHeight = int32(s)
	}

	log.Printf(
		"Starting ingestion from height %d...",
		startHeight,
	)

	p := pipeline.NewPipeline(
		rpcClient,
		dbWriter,
		workers,
		batchSize,
	)

	if err := p.Run(ctx, startHeight); err != nil {
		log.Fatalf("Pipeline error: %v", err)
	}
}

func intEnv(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))

	if err != nil || value < 1 {
		return fallback
	}

	return value
}
