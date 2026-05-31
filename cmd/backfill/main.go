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

// txPartition defines one transactions_* leaf table and its block range.
type txPartition struct {
	txTable  string // e.g. "transactions_200k_300k"
	outTable string // e.g. "tx_outputs_200k_300k"  — used for receiver step
	from     int32
	to       int32
}

// allTxPartitions returns every leaf partition in block-height order.
// Each entry pairs the tx table with its matching tx_outputs table.
var knownPartitions = []txPartition{
	{"transactions_0_100k", "tx_outputs_0_100k", 0, 99999},
	{"transactions_100k_200k", "tx_outputs_100k_200k", 100000, 199999},
	{"transactions_200k_300k", "tx_outputs_200k_300k", 200000, 299999},
	{"transactions_300k_400k", "tx_outputs_300k_400k", 300000, 399999},
	{"transactions_400k_500k", "tx_outputs_400k_500k", 400000, 499999},
	{"transactions_500k_600k", "tx_outputs_500k_600k", 500000, 599999},
	{"transactions_600k_700k", "tx_outputs_600k_700k", 600000, 699999},
	{"transactions_700k_800k", "tx_outputs_700k_800k", 700000, 799999},
	{"transactions_800k_900k", "tx_outputs_800k_900k", 800000, 899999},
	{"transactions_900k_1m", "tx_outputs_900k_1m", 900000, 999999},
	{"transactions_1m_11m", "tx_outputs_1m_11m", 1000000, 1099999},
	{"transactions_default", "tx_outputs_default", 1100000, 9999999},
}

func filteredPartitions(start, end int32) []txPartition {
	var result []txPartition
	for _, p := range knownPartitions {
		if p.to < start || p.from > end {
			continue
		}
		clamped := p
		if clamped.from < start {
			clamped.from = start
		}
		if clamped.to > end {
			clamped.to = end
		}
		result = append(result, clamped)
	}
	return result
}

func loadConfig(path string) (*BackfillConfig, error) {
	cfg := &BackfillConfig{BatchSize: 5000, EndHeight: -1, Workers: 4}
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
	// Workers + 4 headroom for balance/verify queries
	poolCfg.MaxConns = cfg.Workers + 6
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

	if err := verifyPK(ctx, pool); err != nil {
		slog.Error("pk check failed", "err", err)
		os.Exit(1)
	}

	// ── Step 1: Receivers ─────────────────────────────────────────────────────
	// Drives from tx_outputs (by block_height) → joins transactions.
	// Uses idx_tx_brin_* and the tx_outputs PK. Fast because outputs live in
	// the same partition as their transaction.
	if !cfg.SkipReceivers {
		slog.Info("Step 1/4: Receiver address_transactions")
		if err := backfillReceivers(ctx, pool, cfg.StartHeight, end, cfg.BatchSize, cfg.Workers); err != nil {
			slog.Error("receivers failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 1 skipped")
	}

	// ── Step 2: Senders ───────────────────────────────────────────────────────
	// KEY FIX: drive from tx_outputs using spent_height (idx_tx_outputs_*_sh),
	// not from transactions using spending_txid.
	//
	// Old approach: FROM transactions → JOIN tx_outputs ON spending_txid = t.txid
	//   → forces cross-partition scan of all 12 tx_output partitions per batch
	//   → 9-hour stall
	//
	// New approach: FROM tx_outputs WHERE spent_height BETWEEN $1 AND $2
	//   → hits idx_tx_outputs_*_sh index directly (one partition at a time)
	//   → joins back to transactions for block_time / tx_index via txid
	//   → each batch touches 1 tx_output partition × 1 tx partition = fast
	if !cfg.SkipSenders {
		slog.Info("Step 2/4: Sender address_transactions")
		if err := backfillSenders(ctx, pool, cfg.StartHeight, end, cfg.BatchSize, cfg.Workers); err != nil {
			slog.Error("senders failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 2 skipped")
	}

	// ── Step 3: Balances ─────────────────────────────────────────────────────
	// Computed directly from tx_outputs — source of truth, no dependency on
	// address_transactions. Always correct regardless of sender/receiver state.
	if !cfg.SkipBalances {
		slog.Info("Step 3/4: Address balances")
		if err := backfillBalances(ctx, pool, cfg.Workers); err != nil {
			slog.Error("balances failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 3 skipped")
	}

	// ── Step 4: UTXO counts ───────────────────────────────────────────────────
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

// ── verifyPK ─────────────────────────────────────────────────────────────────

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
		return fmt.Errorf("address_transactions has NO primary key")
	}
	hasRole := false
	for i := 0; i+4 <= len(indexdef); i++ {
		if indexdef[i:i+4] == "role" {
			hasRole = true
			break
		}
	}
	if !hasRole {
		return fmt.Errorf("PK does not include role — run:\n" +
			"ALTER TABLE address_transactions DROP CONSTRAINT address_transactions_pkey;\n" +
			"ALTER TABLE address_transactions ADD CONSTRAINT address_transactions_pkey " +
			"PRIMARY KEY (address, block_height, tx_index, txid, role)")
	}
	slog.Info("PK verified — includes role")
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makeJobs[T any](items []T) chan T {
	ch := make(chan T, len(items))
	for _, item := range items {
		ch <- item
	}
	close(ch)
	return ch
}

type heightRange struct{ from, to int32 }

func buildBatches(from, to, batchSize int32) []heightRange {
	var jobs []heightRange
	for cur := from; cur <= to; cur += batchSize {
		next := cur + batchSize - 1
		if next > to {
			next = to
		}
		jobs = append(jobs, heightRange{cur, next})
	}
	return jobs
}

// ── Step 1: Receivers ─────────────────────────────────────────────────────────
//
// Drive from tx_outputs (same partition as transactions).
// Join condition: o.txid = t.txid AND o.block_height = t.block_height
// Both columns are indexed on the same partition → no cross-partition scanning.

func backfillReceivers(ctx context.Context, pool *pgxpool.Pool, start, end, batchSize, workers int32) error {
	partitions := filteredPartitions(start, end)
	grandTotal := int32(len(partitions))
	var grandInserted atomic.Int64

	for pi, p := range partitions {
		slog.Info("receivers partition",
			"table", p.txTable,
			"range", fmt.Sprintf("%d→%d", p.from, p.to),
			"partition", fmt.Sprintf("%d/%d", pi+1, grandTotal),
		)

		jobs := makeJobs(buildBatches(p.from, p.to, batchSize))
		batchCount := int32(len(buildBatches(p.from, p.to, batchSize)))
		partStart := time.Now()

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
					// Drive from tx_outputs partition directly — avoids parent table scan.
					// o.block_height BETWEEN $2 AND $3 uses the BRIN index on tx_outputs.
					query := fmt.Sprintf(`
INSERT INTO address_transactions (
	address, block_height, tx_index, txid, role, net_value_sats, block_time
)
SELECT
	o.address,
	t.block_height,
	t.tx_index,
	t.txid,
	$1,
	SUM(o.value_sats),
	b.block_time
FROM %s o
JOIN %s t  ON t.txid = o.txid
           AND t.block_height = o.block_height
JOIN blocks b ON b.height = t.block_height
WHERE o.block_height BETWEEN $2 AND $3
  AND o.address IS NOT NULL
GROUP BY o.address, t.block_height, t.tx_index, t.txid, b.block_time
ON CONFLICT (address, block_height, tx_index, txid, role) DO NOTHING
`, p.outTable, p.txTable)

					tag, err := pool.Exec(ctx, query, models.RoleReceiver, j.from, j.to)
					if err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = fmt.Errorf("receiver %s %d-%d: %w", p.txTable, j.from, j.to, err)
						}
						mu.Unlock()
						return
					}
					n := done.Add(1)
					ins := inserted.Add(tag.RowsAffected())
					slog.Info("receivers batch",
						"table", p.txTable,
						"range", fmt.Sprintf("%d→%d", j.from, j.to),
						"rows", tag.RowsAffected(),
						"partition_total", ins,
						"batch_progress", fmt.Sprintf("%d/%d", n, batchCount),
						"dur", time.Since(t).Round(time.Millisecond),
					)
				}
			}()
		}

		wg.Wait()
		mu.Lock()
		partErr := firstErr
		mu.Unlock()
		if partErr != nil {
			return partErr
		}

		partRows := inserted.Load()
		grandInserted.Add(partRows)
		slog.Info("receivers partition done",
			"table", p.txTable,
			"rows_inserted", partRows,
			"grand_total", grandInserted.Load(),
			"dur", time.Since(partStart).Round(time.Second),
		)
	}

	slog.Info("receivers done", "total_inserted", grandInserted.Load())
	return nil
}

// ── Step 2: Senders ───────────────────────────────────────────────────────────
//
// THE KEY FIX — drive from tx_outputs using spent_height, NOT from transactions.
//
// Why this works:
//   tx_outputs.spent_height is indexed by idx_tx_outputs_*_sh on EVERY partition.
//   Filtering WHERE spent_height BETWEEN $2 AND $3 hits a single partition's
//   index and returns only the rows spent in that height range.
//   The join back to transactions is a PK lookup (txid) on the matching partition.
//
// Why the old approach failed:
//   FROM transactions → JOIN tx_outputs ON spending_txid = t.txid
//   PostgreSQL must scan all 12 tx_output partitions per transactions batch
//   because spending_txid can point to outputs created at any block height.
//   That's 144 partition combinations → BufferIO stall for 9+ hours.
//
// Partition-by-output-partition strategy:
//   We loop over each tx_outputs partition (by the HEIGHT range the outputs
//   were CREATED in, not spent). Within each, we batch by spent_height so
//   each batch is small and uses the _sh index.
//   The join to transactions uses the explicit tx partition table that matches
//   the spent_height range — this is safe because:
//     spent_height is the block that spent the output = the block the spending
//     tx lives in = the correct tx partition.
//   We therefore need to iterate over ALL tx_output partitions (outputs can
//   be spent at any height), but within each output partition we only touch
//   one tx partition at a time via spent_height batches.

func backfillSenders(ctx context.Context, pool *pgxpool.Pool, start, end, batchSize, workers int32) error {
	// For senders we iterate over ALL output partitions (any output can be spent
	// in our target height range), but we filter by spent_height BETWEEN start AND end.
	allOutPartitions := allOutputPartitions()
	grandTotal := int32(len(allOutPartitions))
	var grandInserted atomic.Int64

	for pi, outP := range allOutPartitions {
		slog.Info("senders output-partition",
			"out_table", outP.table,
			"progress", fmt.Sprintf("%d/%d", pi+1, grandTotal),
		)
		partStart := time.Now()

		// Within this output partition, batch by spent_height in our range.
		// Each batch hits idx_tx_outputs_*_sh for the output table, then
		// looks up the matching tx partition via spent_height → block_height.
		jobs := makeJobs(buildBatches(start, end, batchSize))
		batchCount := int32(len(buildBatches(start, end, batchSize)))

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
					// Drive from this output partition filtered by spent_height.
					// spent_height = the block that spent the output = tx block_height.
					// We join to the transactions PARENT table (uses partition pruning
					// via block_height = o.spent_height which is a constant per row,
					// so PG can prune to the right tx partition automatically).
					// Then join blocks for block_time.
					query := fmt.Sprintf(`
INSERT INTO address_transactions (
	address, block_height, tx_index, txid, role, net_value_sats, block_time
)
SELECT
	o.address,
	o.spent_height             AS block_height,
	t.tx_index,
	o.spending_txid            AS txid,
	$1,
	-SUM(o.value_sats),
	b.block_time
FROM %s o
JOIN transactions t  ON t.txid = o.spending_txid
                    AND t.block_height = o.spent_height
JOIN blocks b        ON b.height = o.spent_height
WHERE o.is_spent        = TRUE
  AND o.spending_txid  IS NOT NULL
  AND o.spent_height   IS NOT NULL
  AND o.address        IS NOT NULL
  AND o.spent_height BETWEEN $2 AND $3
GROUP BY o.address, o.spent_height, t.tx_index, o.spending_txid, b.block_time
ON CONFLICT (address, block_height, tx_index, txid, role) DO NOTHING
`, outP.table)

					tag, err := pool.Exec(ctx, query, models.RoleSender, j.from, j.to)
					if err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = fmt.Errorf("sender %s %d-%d: %w", outP.table, j.from, j.to, err)
						}
						mu.Unlock()
						return
					}
					n := done.Add(1)
					ins := inserted.Add(tag.RowsAffected())
					slog.Info("senders batch",
						"out_table", outP.table,
						"range", fmt.Sprintf("%d→%d", j.from, j.to),
						"rows", tag.RowsAffected(),
						"partition_total", ins,
						"batch_progress", fmt.Sprintf("%d/%d", n, batchCount),
						"dur", time.Since(t).Round(time.Millisecond),
					)
				}
			}()
		}

		wg.Wait()
		mu.Lock()
		partErr := firstErr
		mu.Unlock()
		if partErr != nil {
			return partErr
		}

		partRows := inserted.Load()
		grandInserted.Add(partRows)
		slog.Info("senders output-partition done",
			"out_table", outP.table,
			"rows_inserted", partRows,
			"grand_total", grandInserted.Load(),
			"dur", time.Since(partStart).Round(time.Second),
		)
	}

	slog.Info("senders done", "total_inserted", grandInserted.Load())
	return nil
}

// outputPartition is the tx_outputs leaf table info for sender iteration.
type outputPartition struct {
	table string
}

func allOutputPartitions() []outputPartition {
	return []outputPartition{
		{"tx_outputs_0_100k"},
		{"tx_outputs_100k_200k"},
		{"tx_outputs_200k_300k"},
		{"tx_outputs_300k_400k"},
		{"tx_outputs_400k_500k"},
		{"tx_outputs_500k_600k"},
		{"tx_outputs_600k_700k"},
		{"tx_outputs_700k_800k"},
		{"tx_outputs_800k_900k"},
		{"tx_outputs_900k_1m"},
		{"tx_outputs_1m_11m"},
		{"tx_outputs_default"},
	}
}

// ── Step 3: Balances ──────────────────────────────────────────────────────────
//
// Computed directly from tx_outputs — independent source of truth.
// Prefix-based parallelism avoids full table locks.
// address LIKE $1||'%' uses partial indexes if present, or BRIN otherwise.

func backfillBalances(ctx context.Context, pool *pgxpool.Pool, workers int32) error {
	slog.Info("Truncating address_balances for full rebuild...")
	if _, err := pool.Exec(ctx, "TRUNCATE address_balances"); err != nil {
		return fmt.Errorf("truncate address_balances: %w", err)
	}

	prefixes := addressPrefixes()
	jobs := makeJobs(prefixes)
	total := int32(len(prefixes))
	startTime := time.Now()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var done atomic.Int32

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
	-- received: output created for this address
	SELECT address, txid, block_height,
	       value_sats AS delta, value_sats AS received, 0 AS sent
	FROM tx_outputs
	WHERE address IS NOT NULL
	  AND address LIKE $1 || '%'

	UNION ALL

	-- spent: output from this address consumed by another tx
	SELECT address, spending_txid AS txid, spent_height AS block_height,
	       -value_sats AS delta, 0 AS received, value_sats AS sent
	FROM tx_outputs
	WHERE address IS NOT NULL
	  AND address LIKE $1 || '%'
	  AND is_spent = TRUE
	  AND spending_txid IS NOT NULL
	  AND spent_height IS NOT NULL
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

// ── Step 4: UTXO counts ───────────────────────────────────────────────────────

func backfillUTXOCounts(ctx context.Context, pool *pgxpool.Pool, workers int32) error {
	prefixes := addressPrefixes()
	jobs := makeJobs(prefixes)
	total := int32(len(prefixes))
	startTime := time.Now()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	var done atomic.Int32

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

// ── Verify ────────────────────────────────────────────────────────────────────

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

// ── addressPrefixes ───────────────────────────────────────────────────────────
// Bitcoin addresses use Base58 (no 0, O, I, l).
// Bech32 addresses start with 'b' (bc1...). Taproot also 'b'.
// This covers all realistic first characters.

func addressPrefixes() []string {
	return []string{
		// digits (Base58 — no 0)
		"1", "2", "3", "4", "5", "6", "7", "8", "9",
		// lowercase (Base58 excludes l; bech32 uses a-z)
		"a", "b", "c", "d", "e", "f", "g", "h", "i", "j",
		"k", "m", "n", "o", "p", "q", "r", "s", "t",
		"u", "v", "w", "x", "y", "z",
		// uppercase (Base58 excludes I, O)
		"A", "B", "C", "D", "E", "F", "G", "H", "J",
		"K", "L", "M", "N", "P", "Q", "R", "S", "T",
		"U", "V", "W", "X", "Y", "Z",
	}
}