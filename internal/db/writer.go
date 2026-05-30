package db

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Abhinav7903/Bitcoin-Indexer/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================
// Writer
// ============================================================

type Writer struct {
	pool           *pgxpool.Pool
	historicalSync bool

	// FIX 6: in-memory partition cache — avoids a pg_class query on every
	// batch for partitions we have already confirmed exist.
	partitionCache   map[string]bool
	partitionCacheMu sync.Mutex
}

func NewWriter(pool *pgxpool.Pool, historicalSync bool) *Writer {
	return &Writer{
		pool:           pool,
		historicalSync: historicalSync,
		partitionCache: make(map[string]bool),
	}
}

func (w *Writer) SaveBlockBatch(
	ctx context.Context,
	blocks []models.Block,
	txs []models.Transaction,
	outputs []models.Output,
	addrTxs []models.AddressTransaction,
	inputs []models.Input,
) error {

	if len(blocks) == 0 {
		return nil
	}

	addrTxs = aggregateAddressTransactions(addrTxs)

	if err := w.ensurePartitions(ctx, blocks); err != nil {
		return fmt.Errorf("ensure partitions: %w", err)
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := copyBlocks(ctx, tx, blocks); err != nil {
		return err
	}

	if err := copyTransactions(ctx, tx, txs); err != nil {
		return err
	}

	if err := copyOutputs(ctx, tx, outputs, !w.historicalSync); err != nil {
		return err
	}

	spentInputsReady, err := copyInputs(ctx, tx, inputs)
	if err != nil {
		return err
	}

	if spentInputsReady && !w.historicalSync {
		if err := applySpendState(ctx, tx); err != nil {
			return err
		}
	}

	if !w.historicalSync {

		if err := copyReceiverAddressTransactions(ctx, tx, addrTxs); err != nil {
			return err
		}

		if spentInputsReady {
			if err := copySenderAddressTransactions(ctx, tx); err != nil {
				return err
			}
		}

		if len(addrTxs) > 0 || spentInputsReady {
			if err := updateAddressBalances(ctx, tx, addrTxs, spentInputsReady); err != nil {
				return err
			}
		}
	}

	if err := updateIndexState(ctx, tx, blocks); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ============================================================
// copyBlocks
// ============================================================

func copyBlocks(ctx context.Context, tx pgx.Tx, blocks []models.Block) error {

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"blocks"},
		[]string{
			"height", "hash", "prev_hash", "merkle_root", "block_time",
			"bits", "nonce", "version", "tx_count", "size_bytes",
			"weight", "total_fees_sats",
		},
		pgx.CopyFromSlice(len(blocks), func(i int) ([]interface{}, error) {
			b := blocks[i]
			return []interface{}{
				b.Height, b.Hash, b.PreviousHash, b.MerkleRoot, b.Time,
				b.Bits, b.Nonce, b.Version, b.TxCount, b.SizeBytes,
				b.Weight, b.TotalFeesSats,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy blocks: %w", err)
	}
	return nil
}

// ============================================================
// copyTransactions
// ============================================================

func copyTransactions(ctx context.Context, tx pgx.Tx, txs []models.Transaction) error {

	if len(txs) == 0 {
		return nil
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"transactions"},
		[]string{
			"txid", "block_height", "block_hash", "tx_index", "version",
			"locktime", "is_coinbase", "input_count", "output_count",
			"fee_sats", "size_bytes", "vsize", "weight", "has_segwit",
		},
		pgx.CopyFromSlice(len(txs), func(i int) ([]interface{}, error) {
			t := txs[i]
			return []interface{}{
				t.Txid, t.BlockHeight, t.BlockHash, t.TxIndex, t.Version,
				t.Locktime, t.IsCoinbase, t.InputCount, t.OutputCount,
				t.FeeSats, nullableInt32(t.SizeBytes), nullableInt32(t.VSize),
				nullableInt32(t.Weight), t.HasSegwit,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy transactions: %w", err)
	}
	return nil
}

// ============================================================
// copyOutputs
// ============================================================

func copyOutputs(ctx context.Context, tx pgx.Tx, outputs []models.Output, writeUTXO bool) error {

	if len(outputs) == 0 {
		return nil
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"tx_outputs"},
		[]string{
			"txid", "vout_idx", "block_height", "value_sats",
			"script_pubkey", "script_type", "address",
		},
		pgx.CopyFromSlice(len(outputs), func(i int) ([]interface{}, error) {
			o := outputs[i]
			return []interface{}{
				o.Txid, o.VoutIdx, o.BlockHeight, o.ValueSats,
				nullableBytes(o.ScriptPubKey), o.ScriptType, nullableString(o.Address),
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy tx_outputs: %w", err)
	}

	if !writeUTXO {
		return nil
	}

	utxos := addressOutputs(outputs)
	if len(utxos) == 0 {
		return nil
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"utxo_set"},
		[]string{"txid", "vout_idx", "address", "value_sats", "block_height", "script_type"},
		pgx.CopyFromSlice(len(utxos), func(i int) ([]interface{}, error) {
			o := utxos[i]
			return []interface{}{
				o.Txid, o.VoutIdx, o.Address, o.ValueSats, o.BlockHeight, o.ScriptType,
			}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy utxo_set: %w", err)
	}
	return nil
}

// ============================================================
// copyInputs
// ============================================================

func copyInputs(ctx context.Context, tx pgx.Tx, inputs []models.Input) (bool, error) {

	if len(inputs) == 0 {
		return false, nil
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"tx_inputs"},
		[]string{
			"txid", "vin_idx", "block_height", "prev_txid",
			"prev_vout", "script_sig", "witness_data", "sequence_no",
		},
		pgx.CopyFromSlice(len(inputs), func(i int) ([]interface{}, error) {
			in := inputs[i]
			return []interface{}{
				in.Txid, in.VinIdx, in.BlockHeight,
				nullableBytes(in.PrevTxid), in.PrevVout,
				nullableBytes(in.ScriptSig), in.WitnessData, in.SequenceNo,
			}, nil
		}),
	)
	if err != nil {
		return false, fmt.Errorf("copy tx_inputs: %w", err)
	}

	spendable := spendableInputs(inputs)
	if len(spendable) == 0 {
		return false, nil
	}

	// FIX 9: add source_height column — the block_height of the output being
	// spent. This is the partition key for tx_outputs. Populating it lets
	// PostgreSQL do partition pruning on the UPDATE and every subsequent JOIN,
	// instead of scanning all partitions on every batch.
	if _, err = tx.Exec(ctx, `
CREATE TEMP TABLE temp_spent_inputs (
	prev_txid     BYTEA NOT NULL,
	prev_vout     INT   NOT NULL,
	spending_txid BYTEA NOT NULL,
	spending_vin  INT   NOT NULL,
	spent_height  INT   NOT NULL,
	source_height INT              -- height of the output being spent; filled below
) ON COMMIT DROP
`); err != nil {
		return false, fmt.Errorf("create temp_spent_inputs: %w", err)
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"temp_spent_inputs"},
		[]string{"prev_txid", "prev_vout", "spending_txid", "spending_vin", "spent_height"},
		pgx.CopyFromSlice(len(spendable), func(i int) ([]interface{}, error) {
			in := spendable[i]
			return []interface{}{
				in.PrevTxid, *in.PrevVout, in.Txid, in.VinIdx, in.BlockHeight,
			}, nil
		}),
	)
	if err != nil {
		return false, fmt.Errorf("copy temp_spent_inputs: %w", err)
	}

	// FIX 3: always create the index — even for small batches the join is
	// faster with an index than without. The old threshold of 50 000 meant
	// every normal batch (10-50 blocks) ran hash-join without any index help.
	if _, err = tx.Exec(ctx, `
CREATE INDEX ON temp_spent_inputs (prev_txid, prev_vout)
`); err != nil {
		return false, fmt.Errorf("index temp_spent_inputs: %w", err)
	}

	// FIX 9 (continued): backfill source_height from utxo_set in one pass.
	// utxo_set already stores block_height per UTXO — one UPDATE here saves
	// cross-partition scans in every query that follows.
	if _, err = tx.Exec(ctx, `
UPDATE temp_spent_inputs s
SET    source_height = u.block_height
FROM   utxo_set u
WHERE  u.txid     = s.prev_txid
  AND  u.vout_idx = s.prev_vout
`); err != nil {
		return false, fmt.Errorf("backfill source_height: %w", err)
	}

	// Fresh statistics so the planner picks the right join strategy.
	if _, err = tx.Exec(ctx, `ANALYZE temp_spent_inputs`); err != nil {
		return false, fmt.Errorf("analyze temp_spent_inputs: %w", err)
	}

	return true, nil
}

// ============================================================
// applySpendState
// ============================================================

func applySpendState(ctx context.Context, tx pgx.Tx) error {

	if _, err := tx.Exec(ctx, `
SET LOCAL enable_nestloop  = OFF;
SET LOCAL enable_hashjoin  = ON;
SET LOCAL enable_mergejoin = OFF;
`); err != nil {
		return fmt.Errorf("set join hints: %w", err)
	}

	// FIX 2: add o.block_height = s.source_height to the WHERE clause.
	// tx_outputs is partitioned by block_height. Without this filter,
	// PostgreSQL scans EVERY partition looking for matching (txid, vout_idx).
	// With it, PostgreSQL jumps directly to the one partition that holds each
	// output — turning a full partition-spanning scan into a targeted lookup.
	if _, err := tx.Exec(ctx, `
UPDATE tx_outputs o
SET is_spent      = TRUE,
    spending_txid = s.spending_txid,
    spending_vin  = s.spending_vin,
    spent_height  = s.spent_height
FROM temp_spent_inputs s
WHERE o.txid         = s.prev_txid
  AND o.vout_idx     = s.prev_vout
  AND o.block_height = s.source_height
`); err != nil {
		return fmt.Errorf("mark spent tx_outputs: %w", err)
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM utxo_set u
USING temp_spent_inputs s
WHERE u.txid     = s.prev_txid
  AND u.vout_idx = s.prev_vout
`); err != nil {
		return fmt.Errorf("delete spent utxos: %w", err)
	}

	return nil
}

// ============================================================
// copyReceiverAddressTransactions
// ============================================================

// FIX 1: replaced CopyFrom with SendBatch + ON CONFLICT DO NOTHING.
//
// PostgreSQL COPY does not support ON CONFLICT. Once the PK
// (address, block_height, tx_index, txid, role) exists, any restart
// that reprocesses an already-committed block would crash with:
//   ERROR: duplicate key value violates unique constraint
//
// pgx.SendBatch pipelines all rows in one network round-trip so the
// performance difference vs CopyFrom is negligible for typical batch sizes.
func copyReceiverAddressTransactions(
	ctx context.Context,
	tx pgx.Tx,
	addrTxs []models.AddressTransaction,
) error {

	if len(addrTxs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, a := range addrTxs {
		batch.Queue(`
INSERT INTO address_transactions
	(address, block_height, tx_index, txid, role, net_value_sats, block_time)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (address, block_height, tx_index, txid, role) DO NOTHING
`,
			a.Address, a.BlockHeight, a.TxIndex, a.Txid,
			a.Role, a.NetValueSats, a.BlockTime,
		)
	}

	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(addrTxs); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert address_transaction[%d]: %w", i, err)
		}
	}
	return nil
}

// ============================================================
// copySenderAddressTransactions
// ============================================================

func copySenderAddressTransactions(ctx context.Context, tx pgx.Tx) error {

	// FIX 4: add o.block_height = s.source_height to the tx_outputs JOIN.
	// Without it the JOIN crosses all partitions. With it PostgreSQL prunes
	// to the single partition that holds the spent output.
	//
	// FIX 7: specify conflict columns explicitly instead of bare DO NOTHING.
	_, err := tx.Exec(ctx, `
INSERT INTO address_transactions (
	address, block_height, tx_index, txid, role, net_value_sats, block_time
)
SELECT
	o.address,
	s.spent_height,
	t.tx_index,
	s.spending_txid,
	$1,
	-SUM(o.value_sats),
	b.block_time
FROM temp_spent_inputs s
JOIN tx_outputs o
  ON o.txid         = s.prev_txid
 AND o.vout_idx     = s.prev_vout
 AND o.block_height = s.source_height
JOIN transactions t
  ON t.block_height = s.spent_height
 AND t.txid         = s.spending_txid
JOIN blocks b
  ON b.height = s.spent_height
WHERE o.address IS NOT NULL
GROUP BY
	o.address,
	s.spent_height,
	t.tx_index,
	s.spending_txid,
	b.block_time
ON CONFLICT (address, block_height, tx_index, txid, role) DO NOTHING
`, models.RoleSender)

	if err != nil {
		return fmt.Errorf("copy sender address_transactions: %w", err)
	}
	return nil
}

// ============================================================
// updateAddressBalances
// ============================================================

func updateAddressBalances(
	ctx context.Context,
	tx pgx.Tx,
	receiverRows []models.AddressTransaction,
	includeSenders bool,
) error {

	if _, err := tx.Exec(ctx, `
CREATE TEMP TABLE temp_address_deltas (
	address      TEXT   NOT NULL,
	delta        BIGINT NOT NULL,
	received     BIGINT NOT NULL,
	sent         BIGINT NOT NULL,
	block_height INT    NOT NULL
) ON COMMIT DROP
`); err != nil {
		return fmt.Errorf("create temp_address_deltas: %w", err)
	}

	if len(receiverRows) > 0 {
		_, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"temp_address_deltas"},
			[]string{"address", "delta", "received", "sent", "block_height"},
			pgx.CopyFromSlice(len(receiverRows), func(i int) ([]interface{}, error) {
				a := receiverRows[i]
				return []interface{}{
					a.Address, a.NetValueSats,
					positive(a.NetValueSats), negative(a.NetValueSats),
					a.BlockHeight,
				}, nil
			}),
		)
		if err != nil {
			return fmt.Errorf("copy receiver temp_address_deltas: %w", err)
		}
	}

	if includeSenders {
		// FIX 5: add o.block_height = s.source_height to the JOIN.
		// Same cross-partition issue as copySenderAddressTransactions.
		if _, err := tx.Exec(ctx, `
INSERT INTO temp_address_deltas (address, delta, received, sent, block_height)
SELECT
	o.address,
	-o.value_sats,
	0,
	o.value_sats,
	s.spent_height
FROM temp_spent_inputs s
JOIN tx_outputs o
  ON o.txid         = s.prev_txid
 AND o.vout_idx     = s.prev_vout
 AND o.block_height = s.source_height
WHERE o.address IS NOT NULL
`); err != nil {
			return fmt.Errorf("insert sender temp_address_deltas: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
CREATE INDEX ON temp_address_deltas (address)
`); err != nil {
		return fmt.Errorf("index temp_address_deltas: %w", err)
	}

	if _, err := tx.Exec(ctx, `
CREATE TEMP TABLE temp_address_agg AS
SELECT
	d.address,
	SUM(d.delta)        AS balance_delta,
	SUM(d.received)     AS received_delta,
	SUM(d.sent)         AS sent_delta,
	COUNT(*)            AS tx_delta,
	MIN(d.block_height) AS min_height,
	MAX(d.block_height) AS max_height,
	COALESCE(u.utxo_count, 0) AS utxo_count
FROM (
	SELECT
		address,
		SUM(delta)    AS delta,
		SUM(received) AS received,
		SUM(sent)     AS sent,
		block_height
	FROM temp_address_deltas
	GROUP BY address, block_height
) d
LEFT JOIN (
	SELECT address, COUNT(*)::INT AS utxo_count
	FROM utxo_set
	WHERE address IN (SELECT DISTINCT address FROM temp_address_deltas)
	GROUP BY address
) u ON u.address = d.address
GROUP BY d.address, u.utxo_count
ON COMMIT DROP
`); err != nil {
		return fmt.Errorf("create temp_address_agg: %w", err)
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO address_balances (
	address, balance_sats, total_received_sats, total_sent_sats,
	utxo_count, tx_count, first_seen_height, last_seen_height, updated_at_height
)
SELECT
	address, balance_delta, received_delta, sent_delta,
	utxo_count, tx_delta, min_height, max_height, max_height
FROM temp_address_agg
ON CONFLICT (address) DO UPDATE SET
	balance_sats        = address_balances.balance_sats        + EXCLUDED.balance_sats,
	total_received_sats = address_balances.total_received_sats + EXCLUDED.total_received_sats,
	total_sent_sats     = address_balances.total_sent_sats     + EXCLUDED.total_sent_sats,
	utxo_count          = EXCLUDED.utxo_count,
	tx_count            = address_balances.tx_count            + EXCLUDED.tx_count,
	first_seen_height   = LEAST(
		COALESCE(address_balances.first_seen_height, EXCLUDED.first_seen_height),
		EXCLUDED.first_seen_height
	),
	last_seen_height    = GREATEST(
		COALESCE(address_balances.last_seen_height, EXCLUDED.last_seen_height),
		EXCLUDED.last_seen_height
	),
	updated_at_height   = GREATEST(
		address_balances.updated_at_height,
		EXCLUDED.updated_at_height
	)
`); err != nil {
		return fmt.Errorf("upsert address_balances: %w", err)
	}

	return nil
}

// ============================================================
// updateIndexState
// ============================================================

func updateIndexState(ctx context.Context, tx pgx.Tx, blocks []models.Block) error {

	last := blocks[0]
	for _, b := range blocks[1:] {
		if b.Height > last.Height {
			last = b
		}
	}

	_, err := tx.Exec(ctx, `
INSERT INTO index_state (id, last_indexed_height, last_indexed_hash, updated_at)
VALUES (1, $1, $2, NOW())
ON CONFLICT (id) DO UPDATE SET
	last_indexed_height = EXCLUDED.last_indexed_height,
	last_indexed_hash   = EXCLUDED.last_indexed_hash,
	updated_at          = NOW()
`, last.Height, last.Hash)

	if err != nil {
		return fmt.Errorf("update index_state: %w", err)
	}
	return nil
}

// ============================================================
// ensurePartitions
// ============================================================

func (w *Writer) ensurePartitions(ctx context.Context, blocks []models.Block) error {

	maxHeight := int32(0)
	for _, b := range blocks {
		if b.Height > maxHeight {
			maxHeight = b.Height
		}
	}

	startRange := (maxHeight / 100000) * 100000
	endRange := startRange + 100000

	if maxHeight+1000 >= endRange {
		if err := w.createPartitionIfMissing(ctx, endRange, endRange+100000); err != nil {
			return err
		}
	}

	return w.createPartitionIfMissing(ctx, startRange, endRange)
}

func (w *Writer) createPartitionIfMissing(ctx context.Context, start, end int32) error {

	suffix := fmt.Sprintf("%s_%s", formatPartitionBound(start), formatPartitionBound(end))
	boundExpr := partitionBoundExpr(start, end)

	prefixMap := map[string]string{
		"transactions":         "transactions",
		"tx_outputs":           "tx_outputs",
		"tx_inputs":            "tx_inputs",
		"address_transactions": "address_tx",
	}

	for table, prefix := range prefixMap {

		partitionName := fmt.Sprintf("%s_%s", prefix, suffix)

		// FIX 6: check in-memory cache first — avoids a pg_class query for
		// every batch on partitions we have already confirmed exist.
		w.partitionCacheMu.Lock()
		cached := w.partitionCache[partitionName]
		w.partitionCacheMu.Unlock()

		if cached {
			continue
		}

		var exists bool
		if err := w.pool.QueryRow(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM pg_class child
	JOIN pg_inherits inh ON inh.inhrelid = child.oid
	JOIN pg_class parent ON parent.oid = inh.inhparent
	WHERE parent.relname = $1
	  AND (
		child.relname = $2
		OR pg_get_expr(child.relpartbound, child.oid) = $3
	  )
)
`, table, partitionName, boundExpr).Scan(&exists); err != nil {
			return err
		}

		if !exists {
			slog.Info("Creating partition",
				"partition", partitionName,
				"start", start,
				"end", end,
			)
			q := fmt.Sprintf(
				"CREATE TABLE %s PARTITION OF %s FOR VALUES FROM (%d) TO (%d)",
				partitionName, table, start, end,
			)
			if _, err := w.pool.Exec(ctx, q); err != nil {
				return fmt.Errorf("create partition %s: %w", partitionName, err)
			}
		}

		// Mark as confirmed so subsequent batches skip the DB round-trip.
		w.partitionCacheMu.Lock()
		w.partitionCache[partitionName] = true
		w.partitionCacheMu.Unlock()
	}

	return nil
}

func formatPartitionBound(bound int32) string {
	switch {
	case bound == 0:
		return "0"
	case bound < 1_000_000:
		return fmt.Sprintf("%dk", bound/1000)
	case bound%1_000_000 == 0:
		return fmt.Sprintf("%dm", bound/1_000_000)
	case bound%100_000 == 0:
		return fmt.Sprintf("%dm", bound/100_000)
	default:
		return fmt.Sprintf("%d", bound)
	}
}

func partitionBoundExpr(start, end int32) string {
	return fmt.Sprintf("FOR VALUES FROM (%d) TO (%d)", start, end)
}

// ============================================================
// GetLastHeight
// ============================================================

// FIX 8: distinguish "never indexed" (-1) from "indexed up to block 0" (0).
// The old code returned 0 for both cases, making pipeline.go unable to tell
// whether to start from genesis or from block 1 after a restart.
//
// Callers must handle -1 as "start from genesis (height 0)".
func (w *Writer) GetLastHeight(ctx context.Context) (int32, error) {

	var height int32

	// index_state is the authoritative source — updated atomically with data.
	err := w.pool.QueryRow(
		ctx,
		"SELECT COALESCE(last_indexed_height, -1) FROM index_state WHERE id = 1",
	).Scan(&height)

	if err == nil {
		slog.Info("Last height (from index_state)", "height", height)
		return height, nil
	}

	// Fallback: fresh DB with no index_state row yet.
	err = w.pool.QueryRow(
		ctx,
		"SELECT COALESCE(MAX(height), -1) FROM blocks",
	).Scan(&height)

	slog.Info("Last height (from blocks table)", "height", height)
	return height, err
}

// ============================================================
// Helpers
// ============================================================

func aggregateAddressTransactions(rows []models.AddressTransaction) []models.AddressTransaction {

	if len(rows) < 2 {
		return rows
	}

	type key struct {
		address     string
		blockHeight int32
		txIndex     int32
		txid        string
		role        int16
	}

	out := make([]models.AddressTransaction, 0, len(rows))
	seen := make(map[key]int, len(rows))

	for _, row := range rows {
		k := key{
			address:     row.Address,
			blockHeight: row.BlockHeight,
			txIndex:     row.TxIndex,
			txid:        hex.EncodeToString(row.Txid),
			role:        row.Role,
		}
		if idx, ok := seen[k]; ok {
			out[idx].NetValueSats += row.NetValueSats
			continue
		}
		seen[k] = len(out)
		out = append(out, row)
	}

	return out
}

func addressOutputs(outputs []models.Output) []models.Output {
	filtered := make([]models.Output, 0, len(outputs))
	for _, o := range outputs {
		if o.Address != "" {
			filtered = append(filtered, o)
		}
	}
	return filtered
}

func spendableInputs(inputs []models.Input) []models.Input {
	filtered := make([]models.Input, 0, len(inputs))
	for _, in := range inputs {
		if len(in.PrevTxid) > 0 && in.PrevVout != nil {
			filtered = append(filtered, in)
		}
	}
	return filtered
}

func positive(v int64) int64 {
	if v > 0 {
		return v
	}
	return 0
}

func negative(v int64) int64 {
	if v < 0 {
		return -v
	}
	return 0
}

func nullableBytes(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt32(v int32) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func nullableInt64(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}