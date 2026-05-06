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

	// --------------------------------------------------
	// Graceful shutdown context
	// --------------------------------------------------

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// --------------------------------------------------
	// Load config
	// --------------------------------------------------

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf(
			"Failed to load config: %v",
			err,
		)
	}

	if cfg.DatabaseURL == "" {
		log.Fatal(
			"DATABASE_URL is required",
		)
	}

	if cfg.RPCURL == "" {
		log.Fatal(
			"RPC_URL is required",
		)
	}

	// --------------------------------------------------
	// Parse RPC URL
	// --------------------------------------------------

	u, err := url.Parse(cfg.RPCURL)
	if err != nil {
		log.Fatalf(
			"Invalid RPC_URL: %v",
			err,
		)
	}

	rpcPass, _ := u.User.Password()
	rpcUser := u.User.Username()

	rpcEndpoint := fmt.Sprintf(
		"%s://%s%s",
		u.Scheme,
		u.Host,
		u.Path,
	)

	// --------------------------------------------------
	// PostgreSQL connection pool
	// --------------------------------------------------

	dbConfig, err := pgxpool.ParseConfig(
		cfg.DatabaseURL,
	)
	if err != nil {
		log.Fatalf(
			"Unable to parse DATABASE_URL: %v",
			err,
		)
	}

	pool, err := pgxpool.NewWithConfig(
		ctx,
		dbConfig,
	)
	if err != nil {
		log.Fatalf(
			"Unable to connect to database: %v",
			err,
		)
	}
	defer pool.Close()

	// --------------------------------------------------
	// RPC client
	// --------------------------------------------------

	rpcClient := rpc.NewClient(
		rpcEndpoint,
		rpcUser,
		rpcPass,
	)

	// --------------------------------------------------
	// DB writer
	// --------------------------------------------------

	// historicalSync=true:
	// optimize for fast initial indexing
	dbWriter := db.NewWriter(
		pool,
		true,
	)

	// --------------------------------------------------
	// Determine latest indexed height
	// --------------------------------------------------

	lastHeight, err := dbWriter.GetLastHeight(ctx)
	if err != nil {
		log.Fatalf(
			"Failed to get last indexed height: %v",
			err,
		)
	}

	log.Printf(
		"Latest indexed height: %d",
		lastHeight,
	)

	// --------------------------------------------------
	// Compute start height
	// --------------------------------------------------

	var startHeight int32

	// Empty DB -> genesis
	if lastHeight == 0 {

		var genesisExists bool

		err = pool.QueryRow(
			ctx,
			"SELECT EXISTS(SELECT 1 FROM blocks WHERE height = 0)",
		).Scan(&genesisExists)

		if err != nil {
			log.Fatalf(
				"Failed checking genesis block: %v",
				err,
			)
		}

		if genesisExists {
			startHeight = 1
		} else {
			startHeight = 0
		}

	} else {

		// continue from next block
		startHeight = lastHeight + 1
	}

	// --------------------------------------------------
	// Manual override
	// --------------------------------------------------

	if cfg.StartHeight > 0 {

		log.Printf(
			"Manual start height requested: %d",
			cfg.StartHeight,
		)

		// already indexed
		if cfg.StartHeight <= lastHeight {

			log.Printf(
				"Requested height already indexed | requested=%d latest=%d",
				cfg.StartHeight,
				lastHeight,
			)

			startHeight = lastHeight + 1

			log.Printf(
				"Continuing from next height: %d",
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
			"Continuing from computed start height: %d",
			startHeight,
		)
	}

	log.Printf(
		"Final ingestion start height: %d",
		startHeight,
	)

	// --------------------------------------------------
	// Build ingestion pipeline
	// --------------------------------------------------

	p := pipeline.NewPipeline(
		rpcClient,
		dbWriter,
		cfg.Workers,
		cfg.BatchSize,
	)

	log.Printf(
		"Starting pipeline | workers=%d batch_size=%d",
		cfg.Workers,
		cfg.BatchSize,
	)

	// --------------------------------------------------
	// Run pipeline
	// --------------------------------------------------

	if err := p.Run(ctx, startHeight); err != nil {
		log.Fatalf(
			"Pipeline error: %v",
			err,
		)
	}
}
