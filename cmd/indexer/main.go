package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/Abhinav7903/bitcoin-indexer/internal/config"
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

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required (via config.yaml or environment variable)")
	}

	if cfg.RPCURL == "" {
		log.Fatal("RPC_URL is required (via config.yaml or environment variable)")
	}

	u, err := url.Parse(cfg.RPCURL)
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

	dbConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to parse DATABASE_URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	rpcClient := rpc.NewClient(
		rpcEndpoint,
		rpcUser,
		rpcPass,
	)

	dbWriter := db.NewWriter(pool, true)
	lastHeight, err := dbWriter.GetLastHeight(ctx)
	if err != nil {
		log.Fatalf("Failed to get last height: %v", err)
	}

	startHeight := lastHeight + 1

	// --------------------------------------------------
	// Manual override logic
	// --------------------------------------------------

	if cfg.StartHeight > 0 {

		log.Printf(
			"Manual start height requested: %d",
			cfg.StartHeight,
		)

		// requested height already indexed
		if cfg.StartHeight <= lastHeight {

			log.Printf(
				"Block %d already indexed (DB latest: %d)",
				cfg.StartHeight,
				lastHeight,
			)

			startHeight = lastHeight + 1

			log.Printf(
				"Continuing from latest indexed height: %d",
				startHeight,
			)

		} else {

			startHeight = cfg.StartHeight

			log.Printf(
				"Starting from requested height: %d",
				startHeight,
			)
		}

	} else {

		log.Printf(
			"No manual start height provided",
		)

		log.Printf(
			"Continuing from latest indexed height: %d",
			startHeight,
		)
	}

	log.Printf(
		"Final ingestion start height: %d",
		startHeight,
	)

	p := pipeline.NewPipeline(
		rpcClient,
		dbWriter,
		cfg.Workers,
		cfg.BatchSize,
	)

	if err := p.Run(ctx, startHeight); err != nil {
		log.Fatalf("Pipeline error: %v", err)
	}
}
