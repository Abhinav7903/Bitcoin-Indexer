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
	pool *pgxpool.Pool
}

func NewWriter(pool *pgxpool.Pool) *Writer {
	return &Writer{pool: pool}
}

func (w *Writer) SaveBlockBatch(ctx context.Context, blocks []models.Block, txs []models.Transaction, outputs []models.Output, addrTxs []models.AddressTransaction, inputs []models.Input) error {
	if len(blocks) == 0 {
		return nil
	}
	addrTxs = aggregateAddressTransactions(addrTxs)

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
	if err := copyOutputs(ctx, tx, outputs); err != nil {
		return err
	}
	spentInputsReady, err := copyInputsAndSpendState(ctx, tx, inputs)
	if err != nil {
		return err
	}
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
	if err := updateIndexState(ctx, tx, blocks); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func copyBlocks(ctx context.Context, tx pgx.Tx, blocks []models.Block) error {
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"blocks"}, []string{"height", "hash", "prev_hash", "merkle_root", "block_time", "bits", "nonce", "version", "tx_count", "size_bytes", "weight", "total_fees_sats"}, pgx.CopyFromSlice(len(blocks), func(i int) ([]interface{}, error) {
		b := blocks[i]
		return []interface{}{b.Height, b.Hash, b.PreviousHash, b.MerkleRoot, b.Time, b.Bits, b.Nonce, b.Version, b.TxCount, b.SizeBytes, b.Weight, b.TotalFeesSats}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy blocks: %w", err)
	}
	return nil
}

func copyTransactions(ctx context.Context, tx pgx.Tx, txs []models.Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"transactions"}, []string{"txid", "block_height", "block_hash", "tx_index", "version", "locktime", "is_coinbase", "input_count", "output_count", "fee_sats", "size_bytes", "vsize", "weight", "has_segwit"}, pgx.CopyFromSlice(len(txs), func(i int) ([]interface{}, error) {
		t := txs[i]
		return []interface{}{t.Txid, t.BlockHeight, t.BlockHash, t.TxIndex, t.Version, t.Locktime, t.IsCoinbase, t.InputCount, t.OutputCount, t.FeeSats, nullableInt32(t.SizeBytes), nullableInt32(t.VSize), nullableInt32(t.Weight), t.HasSegwit}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy transactions: %w", err)
	}
	return nil
}

func copyOutputs(ctx context.Context, tx pgx.Tx, outputs []models.Output) error {
	if len(outputs) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"tx_outputs"}, []string{"txid", "vout_idx", "block_height", "value_sats", "script_pubkey", "script_type", "address"}, pgx.CopyFromSlice(len(outputs), func(i int) ([]interface{}, error) {
		o := outputs[i]
		return []interface{}{o.Txid, o.VoutIdx, o.BlockHeight, o.ValueSats, nullableBytes(o.ScriptPubKey), o.ScriptType, nullableString(o.Address)}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy tx_outputs: %w", err)
	}

	utxos := addressOutputs(outputs)
	if len(utxos) == 0 {
		return nil
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"utxo_set"}, []string{"txid", "vout_idx", "address", "value_sats", "block_height", "script_type"}, pgx.CopyFromSlice(len(utxos), func(i int) ([]interface{}, error) {
		o := utxos[i]
		return []interface{}{o.Txid, o.VoutIdx, o.Address, o.ValueSats, o.BlockHeight, o.ScriptType}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy utxo_set: %w", err)
	}
	return nil
}

func copyInputsAndSpendState(ctx context.Context, tx pgx.Tx, inputs []models.Input) (bool, error) {
	if len(inputs) == 0 {
		return false, nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"tx_inputs"}, []string{"txid", "vin_idx", "block_height", "prev_txid", "prev_vout", "script_sig", "witness_data", "sequence_no"}, pgx.CopyFromSlice(len(inputs), func(i int) ([]interface{}, error) {
		in := inputs[i]
		return []interface{}{in.Txid, in.VinIdx, in.BlockHeight, nullableBytes(in.PrevTxid), in.PrevVout, nullableBytes(in.ScriptSig), in.WitnessData, in.SequenceNo}, nil
	}))
	if err != nil {
		return false, fmt.Errorf("copy tx_inputs: %w", err)
	}

	spendInputs := spendableInputs(inputs)
	if len(spendInputs) == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, "CREATE TEMPORARY TABLE temp_spent_inputs (prev_txid BYTEA, prev_vout INT, spending_txid BYTEA, spending_vin INT, spent_height INT) ON COMMIT DROP"); err != nil {
		return false, fmt.Errorf("create temp_spent_inputs: %w", err)
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"temp_spent_inputs"}, []string{"prev_txid", "prev_vout", "spending_txid", "spending_vin", "spent_height"}, pgx.CopyFromSlice(len(spendInputs), func(i int) ([]interface{}, error) {
		in := spendInputs[i]
		return []interface{}{in.PrevTxid, *in.PrevVout, in.Txid, in.VinIdx, in.BlockHeight}, nil
	}))
	if err != nil {
		return false, fmt.Errorf("copy temp_spent_inputs: %w", err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE tx_outputs o
SET is_spent = TRUE,
    spending_txid = s.spending_txid,
    spending_vin = s.spending_vin,
    spent_height = s.spent_height
FROM temp_spent_inputs s
WHERE o.txid = s.prev_txid
  AND o.vout_idx = s.prev_vout`); err != nil {
		return false, fmt.Errorf("mark spent tx_outputs: %w", err)
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM utxo_set u
USING temp_spent_inputs s
WHERE u.txid = s.prev_txid
  AND u.vout_idx = s.prev_vout`); err != nil {
		return false, fmt.Errorf("delete spent utxos: %w", err)
	}
	return true, nil
}

func copyReceiverAddressTransactions(ctx context.Context, tx pgx.Tx, addrTxs []models.AddressTransaction) error {
	if len(addrTxs) == 0 {
		return nil
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"address_transactions"}, []string{"address", "block_height", "tx_index", "txid", "role", "net_value_sats", "block_time"}, pgx.CopyFromSlice(len(addrTxs), func(i int) ([]interface{}, error) {
		a := addrTxs[i]
		return []interface{}{a.Address, a.BlockHeight, a.TxIndex, a.Txid, a.Role, a.NetValueSats, a.BlockTime}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy receiver address_transactions: %w", err)
	}
	return nil
}

func copySenderAddressTransactions(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
INSERT INTO address_transactions (address, block_height, tx_index, txid, role, net_value_sats, block_time)
SELECT o.address,
       s.spent_height,
       t.tx_index,
       s.spending_txid,
       $1,
       -SUM(o.value_sats),
       b.block_time
FROM temp_spent_inputs s
JOIN tx_outputs o ON o.txid = s.prev_txid AND o.vout_idx = s.prev_vout
JOIN transactions t ON t.block_height = s.spent_height AND t.txid = s.spending_txid
JOIN blocks b ON b.height = s.spent_height
WHERE o.address IS NOT NULL
GROUP BY o.address, s.spent_height, t.tx_index, s.spending_txid, b.block_time
ON CONFLICT DO NOTHING`, models.RoleSender)
	if err != nil {
		return fmt.Errorf("copy sender address_transactions: %w", err)
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

func updateAddressBalances(ctx context.Context, tx pgx.Tx, receiverRows []models.AddressTransaction, includeSenders bool) error {
	if _, err := tx.Exec(ctx, "CREATE TEMPORARY TABLE temp_address_deltas (address TEXT, delta BIGINT, received BIGINT, sent BIGINT, block_height INT) ON COMMIT DROP"); err != nil {
		return fmt.Errorf("create temp_address_deltas: %w", err)
	}
	if len(receiverRows) > 0 {
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"temp_address_deltas"}, []string{"address", "delta", "received", "sent", "block_height"}, pgx.CopyFromSlice(len(receiverRows), func(i int) ([]interface{}, error) {
			a := receiverRows[i]
			return []interface{}{a.Address, a.NetValueSats, positive(a.NetValueSats), negative(a.NetValueSats), a.BlockHeight}, nil
		}))
		if err != nil {
			return fmt.Errorf("copy receiver temp_address_deltas: %w", err)
		}
	}
	if includeSenders {
		if _, err := tx.Exec(ctx, `
INSERT INTO temp_address_deltas (address, delta, received, sent, block_height)
SELECT o.address, -o.value_sats, 0, o.value_sats, s.spent_height
FROM temp_spent_inputs s
JOIN tx_outputs o ON o.txid = s.prev_txid AND o.vout_idx = s.prev_vout
WHERE o.address IS NOT NULL`); err != nil {
			return fmt.Errorf("copy sender temp_address_deltas: %w", err)
		}
	}

	_, err := tx.Exec(ctx, `
INSERT INTO address_balances (address, balance_sats, total_received_sats, total_sent_sats, utxo_count, tx_count, first_seen_height, last_seen_height, updated_at_height)
SELECT d.address,
       SUM(d.delta),
       SUM(d.received),
       SUM(d.sent),
       COALESCE(u.utxo_count, 0),
       COUNT(*),
       MIN(d.block_height),
       MAX(d.block_height),
       MAX(d.block_height)
FROM temp_address_deltas d
LEFT JOIN (
    SELECT address, COUNT(*)::INT AS utxo_count
    FROM utxo_set
    WHERE address IN (SELECT DISTINCT address FROM temp_address_deltas)
    GROUP BY address
) u ON u.address = d.address
GROUP BY d.address, u.utxo_count
ON CONFLICT (address) DO UPDATE SET
    balance_sats = address_balances.balance_sats + EXCLUDED.balance_sats,
    total_received_sats = address_balances.total_received_sats + EXCLUDED.total_received_sats,
    total_sent_sats = address_balances.total_sent_sats + EXCLUDED.total_sent_sats,
    utxo_count = EXCLUDED.utxo_count,
    tx_count = address_balances.tx_count + EXCLUDED.tx_count,
    first_seen_height = LEAST(COALESCE(address_balances.first_seen_height, EXCLUDED.first_seen_height), EXCLUDED.first_seen_height),
    last_seen_height = GREATEST(COALESCE(address_balances.last_seen_height, EXCLUDED.last_seen_height), EXCLUDED.last_seen_height),
    updated_at_height = GREATEST(address_balances.updated_at_height, EXCLUDED.updated_at_height)`)
	if err != nil {
		return fmt.Errorf("upsert address_balances: %w", err)
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
INSERT INTO index_state (id, last_indexed_height, last_indexed_hash, updated_at)
VALUES (1, $1, $2, NOW())
ON CONFLICT (id) DO UPDATE SET
    last_indexed_height = EXCLUDED.last_indexed_height,
    last_indexed_hash = EXCLUDED.last_indexed_hash,
    updated_at = NOW()`, last.Height, last.Hash)
	if err != nil {
		return fmt.Errorf("update index_state: %w", err)
	}
	return nil
}

func (w *Writer) GetLastHeight(ctx context.Context) (int32, error) {
	var height int32
	err := w.pool.QueryRow(ctx, "SELECT COALESCE(last_indexed_height, 0) FROM index_state WHERE id = 1").Scan(&height)
	if err == nil {
		return height, nil
	}
	err = w.pool.QueryRow(ctx, "SELECT COALESCE(MAX(height), 0) FROM blocks").Scan(&height)
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
