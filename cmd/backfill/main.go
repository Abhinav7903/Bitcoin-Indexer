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
}

func loadConfig(path string) (*BackfillConfig, error) {
	cfg := &BackfillConfig{BatchSize: 10000, EndHeight: -1, Workers: 4}
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

	slog.Info("Backfill starting",
		"start_height", cfg.StartHeight,
		"end_height", end,
		"batch_size", cfg.BatchSize,
		"workers", cfg.Workers,
	)
	totalStart := time.Now()

	// Verify PK has role — required for both receiver and sender ON CONFLICT
	if err := verifyPK(ctx, pool); err != nil {
		slog.Error("pk check failed", "err", err)
		os.Exit(1)
	}

	if !cfg.SkipReceivers {
		slog.Info("Step 1/4: Receiver address_transactions")
		if err := backfillReceivers(ctx, pool, cfg.StartHeight, end, cfg.BatchSize, cfg.Workers); err != nil {
			slog.Error("receivers failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 1 skipped")
	}

	if !cfg.SkipSenders {
		slog.Info("Step 2/4: Sender address_transactions")
		if err := backfillSenders(ctx, pool, cfg.StartHeight, end, cfg.BatchSize, cfg.Workers); err != nil {
			slog.Error("senders failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 2 skipped")
	}

	if !cfg.SkipBalances {
		slog.Info("Step 3/4: Address balances")
		if err := backfillBalances(ctx, pool, cfg.Workers); err != nil {
			slog.Error("balances failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 3 skipped")
	}

	if !cfg.SkipUTXO {
		slog.Info("Step 4/4: UTXO counts")
		if err := backfillUTXOCounts(ctx, pool, cfg.Workers); err != nil {
			slog.Error("utxo counts failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 4 skipped")
	}

	slog.Info("Verify")
	verifyBackfill(ctx, pool)

	slog.Info("Backfill complete", "total_duration", time.Since(totalStart).Round(time.Second))
}

// verifyPK checks that the PK includes role — required for separate receiver/sender rows
func verifyPK(ctx context.Context, pool *pgxpool.Pool) error {
	var indexdef string
	err := pool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT indexdef FROM pg_indexes WHERE indexname = 'address_transactions_pkey' LIMIT 1),
			'missing'
		)
	`).Scan(&indexdef)
	if err != nil {
		return fmt.Errorf("check pk: %w", err)
	}

	if indexdef == "missing" {
		return fmt.Errorf("address_transactions has NO primary key — run:\nALTER TABLE address_transactions ADD CONSTRAINT address_transactions_pkey PRIMARY KEY (address, block_height, tx_index, txid, role)")
	}

	hasRole := false
	for i := 0; i+4 <= len(indexdef); i++ {
		if indexdef[i:i+4] == "role" {
			hasRole = true
			break
		}
	}

	if !hasRole {
		return fmt.Errorf("PK does not include role — cannot safely insert separate sender/receiver rows.\nRun:\nALTER TABLE address_transactions DROP CONSTRAINT address_transactions_pkey;\nALTER TABLE address_transactions ADD CONSTRAINT address_transactions_pkey PRIMARY KEY (address, block_height, tx_index, txid, role);")
	}

	slog.Info("PK verified — includes role", "ok", true)
	return nil
}

// makeJobs creates a buffered job channel large enough to hold all jobs
// avoiding the deadlock where main goroutine blocks sending to a full channel
// while workers are blocked waiting to log results
func makeJobs[T any](items []T) chan T {
	ch := make(chan T, len(items))
	for _, item := range items {
		ch <- item
	}
	close(ch)
	return ch
}

func backfillReceivers(ctx context.Context, pool *pgxpool.Pool, start, end, batch, workers int32) error {
	type job struct{ from, to int32 }

	// FIX: pre-build all jobs and close channel before starting workers
	// This avoids the deadlock where main blocks sending while workers block logging
	var allJobs []job
	for cur := start; cur <= end; cur += batch {
		next := cur + batch - 1
		if next > end {
			next = end
		}
		allJobs = append(allJobs, job{cur, next})
	}
	jobs := makeJobs(allJobs)
	total := int32(len(allJobs))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var inserted atomic.Int64
	var done atomic.Int32

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
ON CONFLICT (address, block_height, tx_index, txid, role) DO NOTHING
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

	var allJobs []job
	for cur := start; cur <= end; cur += batch {
		next := cur + batch - 1
		if next > end {
			next = end
		}
		allJobs = append(allJobs, job{cur, next})
	}
	jobs := makeJobs(allJobs)
	total := int32(len(allJobs))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var inserted atomic.Int64
	var done atomic.Int32

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
ON CONFLICT (address, block_height, tx_index, txid, role) DO NOTHING
`, models.RoleSender, j.from, j.to)

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
	jobs := makeJobs(prefixes)
	total := int32(len(prefixes))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var done atomic.Int32
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
				slog.Info("balances",
					"prefix", p,
					"progress", fmt.Sprintf("%d/%d", n, total),
					"dur", time.Since(t).Round(time.Millisecond),
				)
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
	jobs := makeJobs(prefixes)
	total := int32(len(prefixes))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var done atomic.Int32
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
				slog.Info("utxo counts",
					"prefix", p,
					"progress", fmt.Sprintf("%d/%d", n, total),
				)
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
		{"receiver rows (role=0)", fmt.Sprintf("SELECT COUNT(*) FROM address_transactions WHERE role = %d", models.RoleReceiver)},
		{"sender rows (role=1)", fmt.Sprintf("SELECT COUNT(*) FROM address_transactions WHERE role = %d", models.RoleSender)},
		{"negative balances (want 0)", "SELECT COUNT(*) FROM address_balances WHERE balance_sats < 0"},
		{"utxo_set count", "SELECT COUNT(*) FROM utxo_set"},
		{"last_indexed_height", "SELECT last_indexed_height FROM index_state WHERE id = 1"},
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