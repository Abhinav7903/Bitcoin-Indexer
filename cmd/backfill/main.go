package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gopkg.in/yaml.v3"
	"github.com/Abhinav7903/bitcoin-indexer/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BackfillConfig struct {
	DatabaseURL  string `yaml:"database_url"`
	BatchSize    int    `yaml:"batch_size"`
	StartHeight  int    `yaml:"start_height"`
	EndHeight    int    `yaml:"end_height"`
	SkipTxs      bool   `yaml:"skip_txs"`
	SkipBalances bool   `yaml:"skip_balances"`
}

func loadBackfillConfig(path string) (*BackfillConfig, error) {
	cfg := &BackfillConfig{
		BatchSize: 5000,
		EndHeight: -1,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// We use the same yaml parser as the main config
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func main() {
	configPath := flag.String("config", "backfill_config.yaml", "Path to backfill config file")
	flag.Parse()

	cfg, err := loadBackfillConfig(*configPath)
	if err != nil {
		slog.Error("Failed to load backfill config", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.BatchSize > 50000 {
		slog.Warn("Batch size is very large and may cause server instability", "batch", cfg.BatchSize)
	}
	if cfg.BatchSize <= 0 {
		slog.Error("Batch size must be greater than 0")
		os.Exit(1)
	}

	if !cfg.SkipTxs {
		if err := backfillRole1(ctx, pool, int32(cfg.StartHeight), int32(cfg.EndHeight), int32(cfg.BatchSize)); err != nil {
			slog.Error("Role 1 backfill failed", "error", err)
			os.Exit(1)
		}
	}

	if !cfg.SkipBalances {
		if err := backfillBalances(ctx, pool); err != nil {
			slog.Error("Balances backfill failed", "error", err)
			os.Exit(1)
		}
	}

	slog.Info("Backfill completed successfully")
}


func backfillRole1(ctx context.Context, pool *pgxpool.Pool, start, end, batch int32) error {
	if end == -1 {
		err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(height), 0) FROM blocks").Scan(&end)
		if err != nil {
			return fmt.Errorf("failed to get max height: %w", err)
		}
	}

	slog.Info("Starting Role 1 (Sender) backfill", "start", start, "end", end, "batch", batch)

	for current := start; current <= end; current += batch {
		next := current + batch - 1
		if next > end {
			next = end
		}

		startBatch := time.Now()
		tag, err := pool.Exec(ctx, `
			INSERT INTO address_transactions (
				address, 
				block_height, 
				tx_index, 
				txid, 
				role, 
				net_value_sats, 
				block_time
			)
			SELECT 
				o.address, 
				o.spent_height, 
				t.tx_index, 
				o.spending_txid, 
				$1, -- Role 1 (Sender)
				-SUM(o.value_sats), 
				b.block_time
			FROM tx_outputs o
			JOIN transactions t ON t.txid = o.spending_txid AND t.block_height = o.spent_height
			JOIN blocks b ON b.height = o.spent_height
			WHERE o.is_spent = TRUE 
			  AND o.address IS NOT NULL
			  AND o.spent_height BETWEEN $2 AND $3
			GROUP BY o.address, o.spent_height, t.tx_index, o.spending_txid, b.block_time
			ON CONFLICT DO NOTHING
		`, models.RoleSender, current, next)

		if err != nil {
			return fmt.Errorf("failed to backfill batch %d-%d: %w", current, next, err)
		}

		slog.Info("Batch completed", 
			"from", current, 
			"to", next, 
			"inserted", tag.RowsAffected(), 
			"duration", time.Since(startBatch))
	}

	return nil
}

func backfillBalances(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("Starting Address Balances reconstruction (chunked by address prefix)")
	start := time.Now()

	// 1. Clear existing balances
	slog.Info("Truncating address_balances...")
	if _, err := pool.Exec(ctx, "TRUNCATE address_balances"); err != nil {
		return fmt.Errorf("truncate failed: %w", err)
	}

	// 2. Define prefixes for chunking (0-9, a-z, A-Z, plus common bitcoin starts)
	// This covers almost all possible base58/bech32 starts
	prefixes := []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9",
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z",
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
	}

	for _, p := range prefixes {
		prefixStart := time.Now()
		slog.Info("Processing balances for prefix", "prefix", p)

		_, err := pool.Exec(ctx, `
			INSERT INTO address_balances (
				address, 
				balance_sats, 
				total_received_sats, 
				total_sent_sats, 
				tx_count, 
				first_seen_height, 
				last_seen_height, 
				updated_at_height
			)
			SELECT 
				address,
				SUM(delta) as balance_sats,
				SUM(received) as total_received_sats,
				SUM(sent) as total_sent_sats,
				COUNT(DISTINCT txid) as tx_count,
				MIN(block_height),
				MAX(block_height),
				MAX(block_height)
			FROM (
				-- All Received
				SELECT address, txid, block_height, value_sats as delta, value_sats as received, 0 as sent 
				FROM tx_outputs 
				WHERE address LIKE $1 || '%'
				
				UNION ALL
				
				-- All Sent
				SELECT address, spending_txid, spent_height, -value_sats as delta, 0 as received, value_sats as sent 
				FROM tx_outputs 
				WHERE address LIKE $1 || '%' AND is_spent = TRUE
			) combined
			GROUP BY address
		`, p)

		if err != nil {
			return fmt.Errorf("failed prefix %s: %w", p, err)
		}
		slog.Debug("Prefix completed", "prefix", p, "duration", time.Since(prefixStart))
	}

	// 3. Final step: Sync UTXO counts (also chunked for safety)
	slog.Info("Updating UTXO counts...")
	for _, p := range prefixes {
		_, err := pool.Exec(ctx, `
			UPDATE address_balances b
			SET utxo_count = u.cnt
			FROM (
				SELECT address, COUNT(*) as cnt 
				FROM utxo_set 
				WHERE address LIKE $1 || '%'
				GROUP BY address
			) u
			WHERE b.address = u.address AND b.address LIKE $1 || '%'
		`, p)
		if err != nil {
			return fmt.Errorf("utxo count update failed for prefix %s: %w", p, err)
		}
	}

	slog.Info("Balances reconstruction completed", "duration", time.Since(start))
	return nil
}
