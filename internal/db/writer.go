package db

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/Abhinav7903/bitcoin-indexer/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Writer struct {
	pool           *pgxpool.Pool
	historicalSync bool
}

func NewWriter(pool *pgxpool.Pool, historicalSync bool) *Writer {
	return &Writer{
		pool:           pool,
		historicalSync: historicalSync,
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

	// -------------------------------------------------------
	// PERF: Disable synchronous_commit for this session.
	// Safe for historical backfill — worst case on crash we
	// re-index the last batch (idempotent via ON CONFLICT).
	// -------------------------------------------------------
	if _, err := tx.Exec(ctx, "SET LOCAL synchronous_commit = OFF"); err != nil {
		return fmt.Errorf("set synchronous_commit: %w", err)
	}

	// -----------------------------------
	// Immutable blockchain data
	// -----------------------------------

	if err := copyBlocks(ctx, tx, blocks); err != nil {
		return err
	}

	if err := copyTransactions(ctx, tx, txs); err != nil {
		return err
	}

	if err := copyOutputs(ctx, tx, outputs); err != nil {
		return err
	}

	spentInputsReady, err := copyInputs(ctx, tx, inputs)
	if err != nil {
		return err
	}

	// receiver rows are cheap
	if err := copyReceiverAddressTransactions(
		ctx,
		tx,
		addrTxs,
	); err != nil {
		return err
	}

	if spentInputsReady {
		if err := applySpendState(ctx, tx); err != nil {
			return err
		}
	}

	if !w.historicalSync {

		if spentInputsReady {
			if err := copySenderAddressTransactions(
				ctx,
				tx,
			); err != nil {
				return err
			}
		}

		if len(addrTxs) > 0 || spentInputsReady {
			if err := updateAddressBalances(
				ctx,
				tx,
				addrTxs,
				spentInputsReady,
			); err != nil {
				return err
			}
		}
	}

	if err := updateIndexState(ctx, tx, blocks); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func copyBlocks(ctx context.Context, tx pgx.Tx, blocks []models.Block) error {
	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"blocks"},
		[]string{
			"height",
			"hash",
			"prev_hash",
			"merkle_root",
			"block_time",
			"bits",
			"nonce",
			"version",
			"tx_count",
			"size_bytes",
			"weight",
			"total_fees_sats",
		},
		pgx.CopyFromSlice(len(blocks), func(i int) ([]interface{}, error) {
			b := blocks[i]

			return []interface{}{
				b.Height,
				b.Hash,
				b.PreviousHash,
				b.MerkleRoot,
				b.Time,
				b.Bits,
				b.Nonce,
				b.Version,
				b.TxCount,
				b.SizeBytes,
				b.Weight,
				b.TotalFeesSats,
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf("copy blocks: %w", err)
	}

	return nil
}

func copyTransactions(ctx context.Context, tx pgx.Tx, txs []models.Transaction) error {

	if len(txs) == 0 {
		return nil
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"transactions"},
		[]string{
			"txid",
			"block_height",
			"block_hash",
			"tx_index",
			"version",
			"locktime",
			"is_coinbase",
			"input_count",
			"output_count",
			"fee_sats",
			"size_bytes",
			"vsize",
			"weight",
			"has_segwit",
		},
		pgx.CopyFromSlice(len(txs), func(i int) ([]interface{}, error) {

			t := txs[i]

			return []interface{}{
				t.Txid,
				t.BlockHeight,
				t.BlockHash,
				t.TxIndex,
				t.Version,
				t.Locktime,
				t.IsCoinbase,
				t.InputCount,
				t.OutputCount,
				t.FeeSats,
				nullableInt32(t.SizeBytes),
				nullableInt32(t.VSize),
				nullableInt32(t.Weight),
				t.HasSegwit,
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf("copy transactions: %w", err)
	}

	return nil
}

func copyOutputs(ctx context.Context, tx pgx.Tx, outputs []models.Output) error {

	if len(outputs) == 0 {
		return nil
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"tx_outputs"},
		[]string{
			"txid",
			"vout_idx",
			"block_height",
			"value_sats",
			"script_pubkey",
			"script_type",
			"address",
		},
		pgx.CopyFromSlice(len(outputs), func(i int) ([]interface{}, error) {

			o := outputs[i]

			return []interface{}{
				o.Txid,
				o.VoutIdx,
				o.BlockHeight,
				o.ValueSats,
				nullableBytes(o.ScriptPubKey),
				o.ScriptType,
				nullableString(o.Address),
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf("copy tx_outputs: %w", err)
	}

	utxos := addressOutputs(outputs)

	if len(utxos) == 0 {
		return nil
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"utxo_set"},
		[]string{
			"txid",
			"vout_idx",
			"address",
			"value_sats",
			"block_height",
			"script_type",
		},
		pgx.CopyFromSlice(len(utxos), func(i int) ([]interface{}, error) {

			o := utxos[i]

			return []interface{}{
				o.Txid,
				o.VoutIdx,
				o.Address,
				o.ValueSats,
				o.BlockHeight,
				o.ScriptType,
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf("copy utxo_set: %w", err)
	}

	return nil
}

func copyInputs(
	ctx context.Context,
	tx pgx.Tx,
	inputs []models.Input,
) (bool, error) {

	if len(inputs) == 0 {
		return false, nil
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"tx_inputs"},
		[]string{
			"txid",
			"vin_idx",
			"block_height",
			"prev_txid",
			"prev_vout",
			"script_sig",
			"witness_data",
			"sequence_no",
		},
		pgx.CopyFromSlice(len(inputs), func(i int) ([]interface{}, error) {

			in := inputs[i]

			return []interface{}{
				in.Txid,
				in.VinIdx,
				in.BlockHeight,
				nullableBytes(in.PrevTxid),
				in.PrevVout,
				nullableBytes(in.ScriptSig),
				in.WitnessData,
				in.SequenceNo,
			}, nil
		}),
	)

	if err != nil {
		return false, fmt.Errorf("copy tx_inputs: %w", err)
	}

	spendInputs := spendableInputs(inputs)

	if len(spendInputs) == 0 {
		return false, nil
	}

	// -------------------------------------------------------
	// Create temp table for spend lookups.
	// PERF: Add an index on (prev_txid, prev_vout) so the
	// UPDATE and DELETE below use hash joins instead of
	// nested loops when the batch is large.
	// -------------------------------------------------------
	if _, err = tx.Exec(ctx, `
CREATE TEMP TABLE temp_spent_inputs (
	prev_txid    BYTEA NOT NULL,
	prev_vout    INT   NOT NULL,
	spending_txid BYTEA NOT NULL,
	spending_vin  INT  NOT NULL,
	spent_height  INT  NOT NULL
) ON COMMIT DROP
`); err != nil {
		return false, fmt.Errorf(
			"create temp_spent_inputs: %w",
			err,
		)
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"temp_spent_inputs"},
		[]string{
			"prev_txid",
			"prev_vout",
			"spending_txid",
			"spending_vin",
			"spent_height",
		},
		pgx.CopyFromSlice(len(spendInputs), func(i int) ([]interface{}, error) {

			in := spendInputs[i]

			return []interface{}{
				in.PrevTxid,
				*in.PrevVout,
				in.Txid,
				in.VinIdx,
				in.BlockHeight,
			}, nil
		}),
	)

	if err != nil {
		return false, fmt.Errorf(
			"copy temp_spent_inputs: %w",
			err,
		)
	}

	// PERF: Index the temp table so UPDATE tx_outputs and DELETE utxo_set
	// can use an index scan instead of a seq scan on the temp table.
	if _, err = tx.Exec(ctx, `
CREATE INDEX ON temp_spent_inputs (prev_txid, prev_vout)
`); err != nil {
		return false, fmt.Errorf("index temp_spent_inputs: %w", err)
	}

	return true, nil
}

func applySpendState(
	ctx context.Context,
	tx pgx.Tx,
) error {

	// PERF: Use a hash join hint via enable_nestloop=off for this session
	// so Postgres prefers a hash join over nested loops on partitioned tables.
	if _, err := tx.Exec(ctx, "SET LOCAL enable_nestloop = OFF"); err != nil {
		return fmt.Errorf("set enable_nestloop: %w", err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE tx_outputs o
SET is_spent      = TRUE,
    spending_txid = s.spending_txid,
    spending_vin  = s.spending_vin,
    spent_height  = s.spent_height
FROM temp_spent_inputs s
WHERE o.txid     = s.prev_txid
  AND o.vout_idx = s.prev_vout
`); err != nil {
		return fmt.Errorf(
			"mark spent tx_outputs: %w",
			err,
		)
	}

	// Restore planner defaults for subsequent queries.
	if _, err := tx.Exec(ctx, "SET LOCAL enable_nestloop = ON"); err != nil {
		return fmt.Errorf("restore enable_nestloop: %w", err)
	}

	return deleteSpentUTXOs(ctx, tx)
}

func deleteSpentUTXOs(
	ctx context.Context,
	tx pgx.Tx,
) error {

	if _, err := tx.Exec(ctx, `
DELETE FROM utxo_set u
USING temp_spent_inputs s
WHERE u.txid     = s.prev_txid
  AND u.vout_idx = s.prev_vout
`); err != nil {
		return fmt.Errorf(
			"delete spent utxos: %w",
			err,
		)
	}

	return nil
}

func copyReceiverAddressTransactions(ctx context.Context, tx pgx.Tx, addrTxs []models.AddressTransaction) error {

	if len(addrTxs) == 0 {
		return nil
	}

	_, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"address_transactions"},
		[]string{
			"address",
			"block_height",
			"tx_index",
			"txid",
			"role",
			"net_value_sats",
			"block_time",
		},
		pgx.CopyFromSlice(len(addrTxs), func(i int) ([]interface{}, error) {

			a := addrTxs[i]

			return []interface{}{
				a.Address,
				a.BlockHeight,
				a.TxIndex,
				a.Txid,
				a.Role,
				a.NetValueSats,
				a.BlockTime,
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf(
			"copy receiver address_transactions: %w",
			err,
		)
	}

	return nil
}

func copySenderAddressTransactions(ctx context.Context, tx pgx.Tx) error {

	_, err := tx.Exec(ctx, `
INSERT INTO address_transactions (
	address,
	block_height,
	tx_index,
	txid,
	role,
	net_value_sats,
	block_time
)
SELECT o.address,
       s.spent_height,
       t.tx_index,
       s.spending_txid,
       $1,
       -SUM(o.value_sats),
       b.block_time
FROM temp_spent_inputs s
JOIN tx_outputs o
  ON o.txid     = s.prev_txid
 AND o.vout_idx = s.prev_vout
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
ON CONFLICT DO NOTHING
`, models.RoleSender)

	if err != nil {
		return fmt.Errorf(
			"copy sender address_transactions: %w",
			err,
		)
	}

	return nil
}

func aggregateAddressTransactions(rows []models.AddressTransaction) []models.AddressTransaction {

	if len(rows) < 2 {
		return rows
	}

	type addressTxKey struct {
		address     string
		blockHeight int32
		txIndex     int32
		txid        string
		role        int16
	}

	aggregated := make([]models.AddressTransaction, 0, len(rows))
	seen := make(map[addressTxKey]int, len(rows))

	for _, row := range rows {

		key := addressTxKey{
			address:     row.Address,
			blockHeight: row.BlockHeight,
			txIndex:     row.TxIndex,
			txid:        hex.EncodeToString(row.Txid),
			role:        row.Role,
		}

		if idx, ok := seen[key]; ok {
			aggregated[idx].NetValueSats += row.NetValueSats
			continue
		}

		seen[key] = len(aggregated)
		aggregated = append(aggregated, row)
	}

	return aggregated
}

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
		return fmt.Errorf(
			"create temp_address_deltas: %w",
			err,
		)
	}

	if len(receiverRows) > 0 {

		_, err := tx.CopyFrom(
			ctx,
			pgx.Identifier{"temp_address_deltas"},
			[]string{
				"address",
				"delta",
				"received",
				"sent",
				"block_height",
			},
			pgx.CopyFromSlice(len(receiverRows), func(i int) ([]interface{}, error) {

				a := receiverRows[i]

				return []interface{}{
					a.Address,
					a.NetValueSats,
					positive(a.NetValueSats),
					negative(a.NetValueSats),
					a.BlockHeight,
				}, nil
			}),
		)

		if err != nil {
			return fmt.Errorf(
				"copy receiver temp_address_deltas: %w",
				err,
			)
		}
	}

	if includeSenders {

		if _, err := tx.Exec(ctx, `
INSERT INTO temp_address_deltas (
	address,
	delta,
	received,
	sent,
	block_height
)
SELECT o.address,
       -o.value_sats,
       0,
       o.value_sats,
       s.spent_height
FROM temp_spent_inputs s
JOIN tx_outputs o
  ON o.txid     = s.prev_txid
 AND o.vout_idx = s.prev_vout
WHERE o.address IS NOT NULL
`); err != nil {
			return fmt.Errorf(
				"copy sender temp_address_deltas: %w",
				err,
			)
		}
	}

	// -------------------------------------------------------
	// PERF: Index temp_address_deltas so the utxo_set join
	// below and the GROUP BY use a hash strategy, not nested
	// loops. Also aggregate deltas first so the upsert touches
	// fewer rows in address_balances.
	// -------------------------------------------------------
	if _, err := tx.Exec(ctx, `
CREATE INDEX ON temp_address_deltas (address)
`); err != nil {
		return fmt.Errorf("index temp_address_deltas: %w", err)
	}

	// PERF: Pre-aggregate all deltas per address into a second temp table.
	// This reduces the number of rows that hit the address_balances upsert
	// and avoids a correlated subquery on utxo_set for every row.
	if _, err := tx.Exec(ctx, `
CREATE TEMP TABLE temp_address_agg AS
SELECT
	d.address,
	SUM(d.delta)    AS balance_delta,
	SUM(d.received) AS received_delta,
	SUM(d.sent)     AS sent_delta,
	COUNT(*)        AS tx_delta,
	MIN(d.block_height) AS min_height,
	MAX(d.block_height) AS max_height,
	COALESCE(u.utxo_count, 0) AS utxo_count
FROM (
	SELECT address,
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
	WHERE address IN (
		SELECT DISTINCT address FROM temp_address_deltas
	)
	GROUP BY address
) u ON u.address = d.address
GROUP BY d.address, u.utxo_count
ON COMMIT DROP
`); err != nil {
		return fmt.Errorf("create temp_address_agg: %w", err)
	}

	_, err := tx.Exec(ctx, `
INSERT INTO address_balances (
	address,
	balance_sats,
	total_received_sats,
	total_sent_sats,
	utxo_count,
	tx_count,
	first_seen_height,
	last_seen_height,
	updated_at_height
)
SELECT
	address,
	balance_delta,
	received_delta,
	sent_delta,
	utxo_count,
	tx_delta,
	min_height,
	max_height,
	max_height
FROM temp_address_agg
ON CONFLICT (address)
DO UPDATE SET
	balance_sats =
		address_balances.balance_sats + EXCLUDED.balance_sats,
	total_received_sats =
		address_balances.total_received_sats + EXCLUDED.total_received_sats,
	total_sent_sats =
		address_balances.total_sent_sats + EXCLUDED.total_sent_sats,
	utxo_count = EXCLUDED.utxo_count,
	tx_count =
		address_balances.tx_count + EXCLUDED.tx_count,
	first_seen_height =
		LEAST(
			COALESCE(address_balances.first_seen_height, EXCLUDED.first_seen_height),
			EXCLUDED.first_seen_height
		),
	last_seen_height =
		GREATEST(
			COALESCE(address_balances.last_seen_height, EXCLUDED.last_seen_height),
			EXCLUDED.last_seen_height
		),
	updated_at_height =
		GREATEST(
			address_balances.updated_at_height,
			EXCLUDED.updated_at_height
		)
`)

	if err != nil {
		return fmt.Errorf(
			"upsert address_balances: %w",
			err,
		)
	}

	return nil
}

func updateIndexState(ctx context.Context, tx pgx.Tx, blocks []models.Block) error {

	last := blocks[0]

	for _, block := range blocks[1:] {
		if block.Height > last.Height {
			last = block
		}
	}

	_, err := tx.Exec(ctx, `
INSERT INTO index_state (
	id,
	last_indexed_height,
	last_indexed_hash,
	updated_at
)
VALUES (1, $1, $2, NOW())
ON CONFLICT (id)
DO UPDATE SET
	last_indexed_height = EXCLUDED.last_indexed_height,
	last_indexed_hash   = EXCLUDED.last_indexed_hash,
	updated_at          = NOW()
`, last.Height, last.Hash)

	if err != nil {
		return fmt.Errorf("update index_state: %w", err)
	}

	return nil
}

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
	suffix := fmt.Sprintf("%dk_%dk", start/1000, end/1000)
	if start >= 1000000 {
		suffix = fmt.Sprintf("%dk_%dk", start/1000, end/1000)
	}

	tables := []string{"transactions", "tx_outputs", "tx_inputs", "address_transactions"}
	prefixMap := map[string]string{
		"transactions":         "transactions",
		"tx_outputs":           "tx_outputs",
		"tx_inputs":            "tx_inputs",
		"address_transactions": "address_tx",
	}

	for _, table := range tables {
		partitionName := fmt.Sprintf("%s_%s", prefixMap[table], suffix)

		var exists bool
		err := w.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_name = $1
			)
		`, partitionName).Scan(&exists)
		if err != nil {
			return err
		}

		if !exists {
			slog.Info("Creating missing partition", "table", table, "partition", partitionName, "start", start, "end", end)
			query := fmt.Sprintf(
				"CREATE TABLE %s PARTITION OF %s FOR VALUES FROM (%d) TO (%d)",
				partitionName, table, start, end,
			)
			if _, err := w.pool.Exec(ctx, query); err != nil {
				return fmt.Errorf("failed to create partition %s: %w", partitionName, err)
			}
		}
	}

	return nil
}

func (w *Writer) GetLastHeight(ctx context.Context) (int32, error) {

	var height int32

	err := w.pool.QueryRow(
		ctx,
		"SELECT COALESCE(last_indexed_height, 0) FROM index_state WHERE id = 1",
	).Scan(&height)

	if err == nil {
		return height, nil
	}

	err = w.pool.QueryRow(
		ctx,
		"SELECT COALESCE(MAX(height), 0) FROM blocks",
	).Scan(&height)

	slog.Info("Last height", "height", height)

	return height, err
}

func addressOutputs(outputs []models.Output) []models.Output {

	filtered := make([]models.Output, 0, len(outputs))

	for _, out := range outputs {
		if out.Address != "" {
			filtered = append(filtered, out)
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

func positive(value int64) int64 {
	if value > 0 {
		return value
	}
	return 0
}

func negative(value int64) int64 {
	if value < 0 {
		return -value
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