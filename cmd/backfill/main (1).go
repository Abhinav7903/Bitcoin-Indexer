package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Abhinav7903/Bitcoin-Indexer/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

type BackfillConfig struct {
	DatabaseURL   string `yaml:"database_url"`
	BatchSize     int32  `yaml:"batch_size"`
	StartHeight   int32  `yaml:"start_height"`
	EndHeight     int32  `yaml:"end_height"`
	Workers       int32  `yaml:"workers"`
	SkipReceivers bool   `yaml:"skip_receivers"`
	SkipSenders   bool   `yaml:"skip_senders"`
	SkipBalances  bool   `yaml:"skip_balances"`
	SkipUTXO      bool   `yaml:"skip_utxo_counts"`
	SkipPKFix     bool   `yaml:"skip_pk_fix"`
}

func loadConfig(path string) (*BackfillConfig, error) {
	cfg := &BackfillConfig{BatchSize: 2000, EndHeight: -1, Workers: 6}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return cfg, yaml.Unmarshal(data, cfg)
}

func main() {
	cfgPath := flag.String("config", "backfill_config.yaml", "path to backfill config")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		slog.Error("parse db url", "err", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = cfg.Workers + 4
	poolCfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		slog.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	end := cfg.EndHeight
	if end == -1 {
		if err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(height),0) FROM blocks").Scan(&end); err != nil {
			slog.Error("get max height", "err", err)
			os.Exit(1)
		}
	}

	slog.Info("Backfill starting", "start_height", cfg.StartHeight, "end_height", end, "batch_size", cfg.BatchSize, "workers", cfg.Workers)
	totalStart := time.Now()

	if !cfg.SkipPKFix {
		slog.Info("Step 0/5: Fix address_transactions PK")
		if err := fixAddressTxPK(ctx, pool); err != nil {
			slog.Error("pk fix failed", "err", err)
			os.Exit(1)
		}
	}

	if !cfg.SkipReceivers {
		slog.Info("Step 1/5: Receiver address_transactions")
		if err := backfillReceivers(ctx, pool, cfg.StartHeight, end, cfg.BatchSize, cfg.Workers); err != nil {
			slog.Error("receivers failed", "err", err)
			os.Exit(1)
		}
	}

	if !cfg.SkipSenders {
		slog.Info("Step 2/5: Sender address_transactions")
		if err := backfillSenders(ctx, pool, cfg.StartHeight, end, cfg.BatchSize, cfg.Workers); err != nil {
			slog.Error("senders failed", "err", err)
			os.Exit(1)
		}
	}

	if !cfg.SkipBalances {
		slog.Info("Step 3/5: Address balances")
		if err := backfillBalances(ctx, pool, cfg.Workers); err != nil {
			slog.Error("balances failed", "err", err)
			os.Exit(1)
		}
	}

	if !cfg.SkipUTXO {
		slog.Info("Step 4/5: UTXO counts")
		if err := backfillUTXOCounts(ctx, pool, cfg.Workers); err != nil {
			slog.Error("utxo counts failed", "err", err)
			os.Exit(1)
		}
	}

	slog.Info("Step 5/5: Verify")
	verifyBackfill(ctx, pool)

	slog.Info("Backfill complete", "total_duration", time.Since(totalStart).Round(time.Second))
}

func fixAddressTxPK(ctx context.Context, pool *pgxpool.Pool) error {
	var indexdef string
	err := pool.QueryRow(ctx, `
		SELECT indexdef FROM pg_indexes
		WHERE indexname = 'address_transactions_pkey' LIMIT 1
	`).Scan(&indexdef)
	if err != nil {
		return fmt.Errorf("check pk: %w", err)
	}

	// Check if role is present in index definition
	hasRole := false
	for i := 0; i+4 <= len(indexdef); i++ {
		if indexdef[i:i+4] == "role" {
			hasRole = true
			break
		}
	}
	if !hasRole {
		slog.Info("address_transactions PK already correct — skipping")
		return nil
	}

	slog.Info("Dropping old PK (has role)...")
	start := time.Now()

	if _, err := pool.Exec(ctx, `ALTER TABLE address_transactions DROP CONSTRAINT address_transactions_pkey`); err != nil {
		return fmt.Errorf("drop old pk: %w", err)
	}

	slog.Info("Rebuilding PK without role...", "elapsed", time.Since(start).Round(time.Second))

	if _, err := pool.Exec(ctx, `
		ALTER TABLE address_transactions
		ADD CONSTRAINT address_transactions_pkey
		PRIMARY KEY (address, block_height, tx_index, txid)
	`); err != nil {
		return fmt.Errorf("add new pk: %w", err)
	}

	slog.Info("PK fixed", "duration", time.Since(start).Round(time.Second))
	return nil
}

func backfillReceivers(ctx context.Context, pool *pgxpool.Pool, start, end, batch, workers int32) error {
	type job struct{ from, to int32 }
	jobs := make(chan job, 128)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var inserted atomic.Int64
	var done atomic.Int32
	total := (end-start)/batch + 1

	for i := int32(0); i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				t := time.Now()
				tag, err := pool.Exec(ctx, `
INSERT INTO address_transactions (
	address, block_height, tx_index, txid, role, net_value_sats, block_time
)
SELECT
	o.address,
	o.block_height,
	t.tx_index,
	o.txid,
	$1,
	SUM(o.value_sats),
	b.block_time
FROM tx_outputs o
JOIN transactions t ON t.block_height = o.block_height AND t.txid = o.txid
JOIN blocks b ON b.height = o.block_height
WHERE o.address IS NOT NULL
  AND o.block_height BETWEEN $2 AND $3
GROUP BY o.address, o.block_height, t.tx_index, o.txid, b.block_time
ON CONFLICT (address, block_height, tx_index, txid) DO NOTHING
`, models.RoleReceiver, j.from, j.to)

				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("receiver %d-%d: %w", j.from, j.to, err)
					}
					mu.Unlock()
					return
				}
				n := done.Add(1)
				ins := inserted.Add(tag.RowsAffected())
				slog.Info("receivers",
					"range", fmt.Sprintf("%d→%d", j.from, j.to),
					"rows", tag.RowsAffected(),
					"total", ins,
					"progress", fmt.Sprintf("%d/%d", n, total),
					"dur", time.Since(t).Round(time.Millisecond),
				)
			}
		}()
	}

	for cur := start; cur <= end; cur += batch {
		next := cur + batch - 1
		if next > end {
			next = end
		}
		jobs <- job{cur, next}
	}
	close(jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	slog.Info("receivers done", "total_inserted", inserted.Load())
	return nil
}

func backfillSenders(ctx context.Context, pool *pgxpool.Pool, start, end, batch, workers int32) error {
	type job struct{ from, to int32 }
	jobs := make(chan job, 128)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var inserted atomic.Int64
	var done atomic.Int32
	total := (end-start)/batch + 1

	for i := int32(0); i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				t := time.Now()
				tag, err := pool.Exec(ctx, `
INSERT INTO address_transactions (
	address, block_height, tx_index, txid, role, net_value_sats, block_time
)
SELECT
	o.address,
	o.spent_height,
	t.tx_index,
	o.spending_txid,
	$1,
	-SUM(o.value_sats),
	b.block_time
FROM tx_outputs o
JOIN transactions t ON t.block_height = o.spent_height AND t.txid = o.spending_txid
JOIN blocks b ON b.height = o.spent_height
WHERE o.is_spent      = TRUE
  AND o.address       IS NOT NULL
  AND o.spending_txid IS NOT NULL
  AND o.spent_height  IS NOT NULL
  AND o.spent_height  BETWEEN $2 AND $3
GROUP BY o.address, o.spent_height, t.tx_index, o.spending_txid, b.block_time
ON CONFLICT (address, block_height, tx_index, txid) DO UPDATE SET
	role = CASE
	           WHEN address_transactions.role = $4 THEN $5
	           ELSE EXCLUDED.role
	       END,
	net_value_sats = address_transactions.net_value_sats + EXCLUDED.net_value_sats
`, models.RoleSender, j.from, j.to, models.RoleReceiver, models.RoleBoth)

				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("sender %d-%d: %w", j.from, j.to, err)
					}
					mu.Unlock()
					return
				}
				n := done.Add(1)
				ins := inserted.Add(tag.RowsAffected())
				slog.Info("senders",
					"range", fmt.Sprintf("%d→%d", j.from, j.to),
					"rows", tag.RowsAffected(),
					"total", ins,
					"progress", fmt.Sprintf("%d/%d", n, total),
					"dur", time.Since(t).Round(time.Millisecond),
				)
			}
		}()
	}

	for cur := start; cur <= end; cur += batch {
		next := cur + batch - 1
		if next > end {
			next = end
		}
		jobs <- job{cur, next}
	}
	close(jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	slog.Info("senders done", "total_inserted", inserted.Load())
	return nil
}

func backfillBalances(ctx context.Context, pool *pgxpool.Pool, workers int32) error {
	slog.Info("Truncating address_balances for full rebuild...")
	if _, err := pool.Exec(ctx, "TRUNCATE address_balances"); err != nil {
		return fmt.Errorf("truncate address_balances: %w", err)
	}

	prefixes := addressPrefixes()
	jobs := make(chan string, len(prefixes))
	for _, p := range prefixes {
		jobs <- p
	}
	close(jobs)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var done atomic.Int32
	total := int32(len(prefixes))
	startTime := time.Now()

	for i := int32(0); i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				t := time.Now()
				_, err := pool.Exec(ctx, `
INSERT INTO address_balances (
	address, balance_sats, total_received_sats, total_sent_sats,
	tx_count, first_seen_height, last_seen_height, updated_at_height
)
SELECT
	address,
	SUM(delta)           AS balance_sats,
	SUM(received)        AS total_received_sats,
	SUM(sent)            AS total_sent_sats,
	COUNT(DISTINCT txid) AS tx_count,
	MIN(block_height)    AS first_seen_height,
	MAX(block_height)    AS last_seen_height,
	MAX(block_height)    AS updated_at_height
FROM (
	SELECT address, txid, block_height,
	       value_sats AS delta, value_sats AS received, 0 AS sent
	FROM tx_outputs
	WHERE address IS NOT NULL AND address LIKE $1 || '%'

	UNION ALL

	SELECT address, spending_txid AS txid, spent_height AS block_height,
	       -value_sats AS delta, 0 AS received, value_sats AS sent
	FROM tx_outputs
	WHERE address IS NOT NULL AND address LIKE $1 || '%'
	  AND is_spent = TRUE AND spending_txid IS NOT NULL AND spent_height IS NOT NULL
) combined
GROUP BY address
ON CONFLICT (address) DO UPDATE SET
	balance_sats        = EXCLUDED.balance_sats,
	total_received_sats = EXCLUDED.total_received_sats,
	total_sent_sats     = EXCLUDED.total_sent_sats,
	tx_count            = EXCLUDED.tx_count,
	first_seen_height   = LEAST(address_balances.first_seen_height, EXCLUDED.first_seen_height),
	last_seen_height    = GREATEST(address_balances.last_seen_height, EXCLUDED.last_seen_height),
	updated_at_height   = GREATEST(address_balances.updated_at_height, EXCLUDED.updated_at_height)
`, p)

				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("balance prefix %s: %w", p, err)
					}
					mu.Unlock()
					return
				}
				n := done.Add(1)
				slog.Info("balances", "prefix", p, "progress", fmt.Sprintf("%d/%d", n, total), "dur", time.Since(t).Round(time.Millisecond))
			}
		}()
	}

	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	slog.Info("balances done", "duration", time.Since(startTime).Round(time.Second))
	return nil
}

func backfillUTXOCounts(ctx context.Context, pool *pgxpool.Pool, workers int32) error {
	prefixes := addressPrefixes()
	jobs := make(chan string, len(prefixes))
	for _, p := range prefixes {
		jobs <- p
	}
	close(jobs)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var done atomic.Int32
	total := int32(len(prefixes))
	startTime := time.Now()

	for i := int32(0); i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				_, err := pool.Exec(ctx, `
UPDATE address_balances b
SET    utxo_count = u.cnt
FROM (
	SELECT address, COUNT(*)::INT AS cnt
	FROM   utxo_set
	WHERE  address LIKE $1 || '%'
	GROUP  BY address
) u
WHERE b.address = u.address
  AND b.address LIKE $1 || '%'
`, p)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("utxo prefix %s: %w", p, err)
					}
					mu.Unlock()
					return
				}
				n := done.Add(1)
				slog.Info("utxo counts", "prefix", p, "progress", fmt.Sprintf("%d/%d", n, total))
			}
		}()
	}

	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	slog.Info("utxo counts done", "duration", time.Since(startTime).Round(time.Second))
	return nil
}

func verifyBackfill(ctx context.Context, pool *pgxpool.Pool) {
	checks := []struct {
		name  string
		query string
	}{
		{"address_balances count", "SELECT COUNT(*) FROM address_balances"},
		{"address_transactions count", "SELECT COUNT(*) FROM address_transactions"},
		{"negative balances (want 0)", "SELECT COUNT(*) FROM address_balances WHERE balance_sats < 0"},
		{"utxo_set count", "SELECT COUNT(*) FROM utxo_set"},
		{"last_indexed_height", "SELECT last_indexed_height FROM index_state WHERE id = 1"},
		{"self-send rows (role=2)", fmt.Sprintf("SELECT COUNT(*) FROM address_transactions WHERE role = %d", models.RoleBoth)},
	}

	for _, c := range checks {
		var val int64
		if err := pool.QueryRow(ctx, c.query).Scan(&val); err != nil {
			slog.Warn("verify", "check", c.name, "err", err)
			continue
		}
		slog.Info("verify", "check", c.name, "value", val)
	}
}

func addressPrefixes() []string {
	return []string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9",
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j",
		"k", "l", "m", "n", "o", "p", "q", "r", "s", "t",
		"u", "v", "w", "x", "y", "z",
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J",
		"K", "L", "M", "N", "O", "P", "Q", "R", "S", "T",
		"U", "V", "W", "X", "Y", "Z",
	}
}
