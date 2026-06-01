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
	SkipSpend     bool   `yaml:"skip_spend"`
	SkipReceivers bool   `yaml:"skip_receivers"`
	SkipSenders   bool   `yaml:"skip_senders"`
	SkipBalances  bool   `yaml:"skip_balances"`
	SkipUTXO      bool   `yaml:"skip_utxo_counts"`
}

type partition struct {
	txTable  string
	outTable string
	inTable  string
	from     int32
	to       int32
}

var allPartitions = []partition{
	{"transactions_0_100k",    "tx_outputs_0_100k",    "tx_inputs_0_100k",    0,       99999},
	{"transactions_100k_200k", "tx_outputs_100k_200k", "tx_inputs_100k_200k", 100000,  199999},
	{"transactions_200k_300k", "tx_outputs_200k_300k", "tx_inputs_200k_300k", 200000,  299999},
	{"transactions_300k_400k", "tx_outputs_300k_400k", "tx_inputs_300k_400k", 300000,  399999},
	{"transactions_400k_500k", "tx_outputs_400k_500k", "tx_inputs_400k_500k", 400000,  499999},
	{"transactions_500k_600k", "tx_outputs_500k_600k", "tx_inputs_500k_600k", 500000,  599999},
	{"transactions_600k_700k", "tx_outputs_600k_700k", "tx_inputs_600k_700k", 600000,  699999},
	{"transactions_700k_800k", "tx_outputs_700k_800k", "tx_inputs_700k_800k", 700000,  799999},
	{"transactions_800k_900k", "tx_outputs_800k_900k", "tx_inputs_800k_900k", 800000,  899999},
	{"transactions_900k_1m",   "tx_outputs_900k_1m",   "tx_inputs_900k_1m",   900000,  999999},
	{"transactions_1m_11m",    "tx_outputs_1m_11m",    "tx_inputs_1m_11m",    1000000, 1099999},
	{"transactions_default",   "tx_outputs_default",   "tx_inputs_default",   1100000, 9999999},
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

func makeJobs[T any](items []T) chan T {
	ch := make(chan T, len(items))
	for _, item := range items {
		ch <- item
	}
	close(ch)
	return ch
}

func filteredPartitions(start, end int32) []partition {
	var result []partition
	for _, p := range allPartitions {
		if p.to < start || p.from > end {
			continue
		}
		c := p
		if c.from < start {
			c.from = start
		}
		if c.to > end {
			c.to = end
		}
		result = append(result, c)
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
	cfgPath := flag.String("config", "backfill_config.yaml", "path to config")
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
		slog.Error("pk check", "err", err)
		os.Exit(1)
	}

	// ── Step 0: Mark Spent ───────────────────────────────────────────────────
	// CORRECT STRATEGY: explicit output_partition × input_partition pairs.
	//
	// For each tx_outputs partition (outputs created at height X):
	//   For each tx_inputs partition (spending txs at height Y):
	//     UPDATE tx_outputs_X SET is_spent=TRUE
	//     FROM tx_inputs_Y i
	//     WHERE i.prev_txid = o.txid AND i.prev_vout = o.vout_idx
	//
	// Why fast:
	//   - tx_outputs_X is explicit → UPDATE stays in ONE partition
	//   - tx_inputs_Y is explicit → idx_txin_prev_Y index used for lookup
	//   - No block_height range filter on tx_inputs → no BRIN/seqscan
	//   - Each pair completes in seconds via index nested loop
	//
	// Total: 12 × 12 = 144 pairs. Most return 0 rows (outputs created
	// in partition X are rarely spent by inputs in far-away partition Y).
	// Only the diagonal and nearby pairs have real rows.
	if !cfg.SkipSpend {
		slog.Info("Step 0/4: Mark spent outputs (partition-pair strategy)")
		if err := backfillSpend(ctx, pool, cfg.StartHeight, end, cfg.Workers); err != nil {
			slog.Error("spend failed", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Step 0 skipped")
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

func verifyPK(ctx context.Context, pool *pgxpool.Pool) error {
	var def string
	_ = pool.QueryRow(ctx, `SELECT COALESCE(
		(SELECT indexdef FROM pg_indexes WHERE indexname='address_transactions_pkey' LIMIT 1),
		'missing')`).Scan(&def)
	if def == "missing" {
		return fmt.Errorf("address_transactions has no PK")
	}
	for i := 0; i+4 <= len(def); i++ {
		if def[i:i+4] == "role" {
			slog.Info("PK verified — includes role")
			return nil
		}
	}
	return fmt.Errorf("PK missing role column")
}

// ── Step 0: Mark Spent ────────────────────────────────────────────────────────
// Explicit output × input partition pairs.
// Each pair: UPDATE one output partition FROM one input partition.
// idx_txin_prev_* index on (prev_txid, prev_vout) makes each pair fast.
// No block_height range on tx_inputs — avoids BRIN scan, uses btree index.

type spendPair struct {
	outTable string
	inTable  string
	pairNum  int
	total    int
}

func backfillSpend(ctx context.Context, pool *pgxpool.Pool, start, end int32, workers int32) error {
	// Build all 144 pairs but only include output partitions that overlap
	// with our target range (outputs that could have been spent in start→end).
	// For spend marking we process ALL output partitions since any output
	// can be spent in our range. Filter by spending input partition range.
	var pairs []spendPair
	for _, outP := range allPartitions {
		for _, inP := range allPartitions {
			// Only include input partitions within our target spending range
			if inP.to < start || inP.from > end {
				continue
			}
			pairs = append(pairs, spendPair{
				outTable: outP.outTable,
				inTable:  inP.inTable,
			})
		}
	}
	// Set pair numbers
	for i := range pairs {
		pairs[i].pairNum = i + 1
		pairs[i].total = len(pairs)
	}

	jobs := makeJobs(pairs)
	var grandUpdated atomic.Int64
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	startTime := time.Now()

	for i := int32(0); i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pair := range jobs {
				t0 := time.Now()

				// THE KEY QUERY:
				// - UPDATE explicit output partition (no cross-partition write)
				// - FROM explicit input partition (idx_txin_prev_* index lookup)
				// - No block_height filter on inputs → pure index nested loop
				// - Only update rows where is_spent=FALSE → idempotent
				query := fmt.Sprintf(`
						UPDATE %s o
						SET
							is_spent      = TRUE,
							spending_txid = i.txid,
							spending_vin  = i.vin_idx,
							spent_height  = i.block_height
						FROM %s i
						WHERE i.prev_txid = o.txid
						AND i.prev_vout = o.vout_idx
						AND o.is_spent  = FALSE
						`, pair.outTable, pair.inTable)

				tag, err := pool.Exec(ctx, query)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("spend %s←%s: %w", pair.outTable, pair.inTable, err)
					}
					mu.Unlock()
					return
				}

				upd := grandUpdated.Add(tag.RowsAffected())
				dur := time.Since(t0).Round(time.Millisecond)

				if tag.RowsAffected() > 0 {
					slog.Info("spend pair",
						"out", pair.outTable,
						"in", pair.inTable,
						"updated", tag.RowsAffected(),
						"grand_total", upd,
						"progress", fmt.Sprintf("%d/%d", pair.pairNum, pair.total),
						"dur", dur,
					)
				} else {
					// Log every 10 empty pairs so we can see progress
					if pair.pairNum%10 == 0 {
						slog.Info("spend progress",
							"progress", fmt.Sprintf("%d/%d", pair.pairNum, pair.total),
							"grand_total", upd,
							"elapsed", time.Since(startTime).Round(time.Second),
						)
					}
				}
			}
		}()
	}

	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	slog.Info("spend done",
		"total_updated", grandUpdated.Load(),
		"duration", time.Since(startTime).Round(time.Second),
	)
	return nil
}

// ── Step 1: Receivers ─────────────────────────────────────────────────────────

func backfillReceivers(ctx context.Context, pool *pgxpool.Pool, start, end, batchSize, workers int32) error {
	parts := filteredPartitions(start, end)
	grandTotal := int32(len(parts))
	var grandInserted atomic.Int64

	for pi, p := range parts {
		slog.Info("receivers partition",
			"table", p.txTable,
			"range", fmt.Sprintf("%d→%d", p.from, p.to),
			"progress", fmt.Sprintf("%d/%d", pi+1, grandTotal),
		)
		partStart := time.Now()
		jobs := makeJobs(buildBatches(p.from, p.to, batchSize))
		batchCount := int32(len(buildBatches(p.from, p.to, batchSize)))

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
					t0 := time.Now()
					query := fmt.Sprintf(`
INSERT INTO address_transactions (
	address, block_height, tx_index, txid, role, net_value_sats, block_time
)
SELECT o.address, t.block_height, t.tx_index, t.txid, $1,
	SUM(o.value_sats), b.block_time
FROM %s o
JOIN %s t  ON t.txid = o.txid AND t.block_height = o.block_height
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
						"progress", fmt.Sprintf("%d/%d", n, batchCount),
						"dur", time.Since(t0).Round(time.Millisecond),
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
			"table", p.txTable, "rows_inserted", partRows,
			"grand_total", grandInserted.Load(),
			"dur", time.Since(partStart).Round(time.Second),
		)
	}
	slog.Info("receivers done", "total_inserted", grandInserted.Load())
	return nil
}

// ── Step 2: Senders ───────────────────────────────────────────────────────────
// Drive from explicit tx_inputs partition.
// Join tx_outputs via PK — after Step 0, every spent output has is_spent=TRUE.
// tx_outputs PK is (block_height, txid, vout_idx) — PG uses it for lookup.

func backfillSenders(ctx context.Context, pool *pgxpool.Pool, start, end, batchSize, workers int32) error {
	parts := filteredPartitions(start, end)
	grandTotal := int32(len(parts))
	var grandInserted atomic.Int64

	for pi, p := range parts {
		slog.Info("senders partition",
			"table", p.inTable,
			"range", fmt.Sprintf("%d→%d", p.from, p.to),
			"progress", fmt.Sprintf("%d/%d", pi+1, grandTotal),
		)
		partStart := time.Now()
		jobs := makeJobs(buildBatches(p.from, p.to, batchSize))
		batchCount := int32(len(buildBatches(p.from, p.to, batchSize)))

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
					t0 := time.Now()
					query := fmt.Sprintf(`
INSERT INTO address_transactions (
	address, block_height, tx_index, txid, role, net_value_sats, block_time
)
SELECT
	o.address,
	i.block_height,
	t.tx_index,
	i.txid,
	$1,
	-SUM(o.value_sats),
	b.block_time
FROM %s i
JOIN tx_outputs o
  ON o.txid     = i.prev_txid
 AND o.vout_idx = i.prev_vout
JOIN transactions t
  ON t.txid         = i.txid
 AND t.block_height = i.block_height
JOIN blocks b ON b.height = i.block_height
WHERE i.block_height BETWEEN $2 AND $3
  AND i.prev_txid IS NOT NULL
  AND o.address   IS NOT NULL
GROUP BY o.address, i.block_height, t.tx_index, i.txid, b.block_time
ON CONFLICT (address, block_height, tx_index, txid, role) DO NOTHING
`, p.inTable)

					tag, err := pool.Exec(ctx, query, models.RoleSender, j.from, j.to)
					if err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = fmt.Errorf("sender %s %d-%d: %w", p.inTable, j.from, j.to, err)
						}
						mu.Unlock()
						return
					}
					n := done.Add(1)
					ins := inserted.Add(tag.RowsAffected())
					slog.Info("senders batch",
						"table", p.inTable,
						"range", fmt.Sprintf("%d→%d", j.from, j.to),
						"rows", tag.RowsAffected(),
						"partition_total", ins,
						"progress", fmt.Sprintf("%d/%d", n, batchCount),
						"dur", time.Since(t0).Round(time.Millisecond),
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
		slog.Info("senders partition done",
			"table", p.inTable, "rows_inserted", partRows,
			"grand_total", grandInserted.Load(),
			"dur", time.Since(partStart).Round(time.Second),
		)
	}
	slog.Info("senders done", "total_inserted", grandInserted.Load())
	return nil
}

// ── Step 3: Balances ──────────────────────────────────────────────────────────

func backfillBalances(ctx context.Context, pool *pgxpool.Pool, workers int32) error {
	slog.Info("Truncating address_balances...")
	if _, err := pool.Exec(ctx, "TRUNCATE address_balances"); err != nil {
		return fmt.Errorf("truncate: %w", err)
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
				t0 := time.Now()
				_, err := pool.Exec(ctx, `
INSERT INTO address_balances (
	address, balance_sats, total_received_sats, total_sent_sats,
	tx_count, first_seen_height, last_seen_height, updated_at_height
)
SELECT address,
	SUM(delta) AS balance_sats,
	SUM(received) AS total_received_sats,
	SUM(sent) AS total_sent_sats,
	COUNT(DISTINCT txid) AS tx_count,
	MIN(block_height) AS first_seen_height,
	MAX(block_height) AS last_seen_height,
	MAX(block_height) AS updated_at_height
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
				slog.Info("balances", "prefix", p,
					"progress", fmt.Sprintf("%d/%d", n, total),
					"dur", time.Since(t0).Round(time.Millisecond))
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
UPDATE address_balances b SET utxo_count = u.cnt
FROM (
	SELECT address, COUNT(*)::INT AS cnt
	FROM utxo_set WHERE address LIKE $1 || '%'
	GROUP BY address
) u
WHERE b.address = u.address AND b.address LIKE $1 || '%'
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
				slog.Info("utxo counts", "prefix", p,
					"progress", fmt.Sprintf("%d/%d", n, total))
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
	checks := []struct{ name, query string }{
		{"address_balances count",     "SELECT COUNT(*) FROM address_balances"},
		{"address_transactions count", "SELECT COUNT(*) FROM address_transactions"},
		{"receiver rows (role=0)",     fmt.Sprintf("SELECT COUNT(*) FROM address_transactions WHERE role=%d", models.RoleReceiver)},
		{"sender rows (role=1)",       fmt.Sprintf("SELECT COUNT(*) FROM address_transactions WHERE role=%d", models.RoleSender)},
		{"negative balances (want 0)", "SELECT COUNT(*) FROM address_balances WHERE balance_sats < 0"},
		{"spent outputs total",        "SELECT COUNT(*) FROM tx_outputs WHERE is_spent=TRUE"},
		{"utxo_set count",             "SELECT COUNT(*) FROM utxo_set"},
		{"last_indexed_height",        "SELECT last_indexed_height FROM index_state WHERE id=1"},
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
		"k", "m", "n", "o", "p", "q", "r", "s", "t",
		"u", "v", "w", "x", "y", "z",
		"A", "B", "C", "D", "E", "F", "G", "H", "J",
		"K", "L", "M", "N", "P", "Q", "R", "S", "T",
		"U", "V", "W", "X", "Y", "Z",
	}
}