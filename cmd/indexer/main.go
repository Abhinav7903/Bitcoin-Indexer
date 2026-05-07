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
	// PostgreSQL
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

	dbWriter := db.NewWriter(
		pool,
		true,
	)

	// --------------------------------------------------
	// Latest indexed height
	// --------------------------------------------------

	lastHeight, err := dbWriter.GetLastHeight(ctx)
	if err != nil {
		log.Fatalf(
			"Failed to get last height: %v",
			err,
		)
	}

	log.Printf(
		"Latest indexed height: %d",
		lastHeight,
	)

	// --------------------------------------------------
	// Correct start height logic
	// --------------------------------------------------

	var startHeight int32

	// Empty DB -> genesis block
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

		// Continue from next block
		startHeight = lastHeight + 1
	}

	// --------------------------------------------------
	// Manual override
	// --------------------------------------------------

	if cfg.StartHeight > 0 {

		startHeight = cfg.StartHeight

		log.Printf(
			"Manual override start height: %d",
			startHeight,
		)
	}

	log.Printf(
		"Starting ingestion from height %d...",
		startHeight,
	)

	// --------------------------------------------------
	// Pipeline
	// --------------------------------------------------

	p := pipeline.NewPipeline(
		rpcClient,
		dbWriter,
		cfg.Workers,
		cfg.BatchSize,
	)

	if err := p.Run(ctx, startHeight); err != nil {
		log.Fatalf(
			"Pipeline error: %v",
			err,
		)
	}
}
