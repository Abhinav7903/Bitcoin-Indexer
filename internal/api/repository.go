package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Abhinav7903/bitcoin-indexer/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetAddressInfo(ctx context.Context, address string) (*AddressInfo, error) {
	var info AddressInfo
	err := r.pool.QueryRow(ctx, `
		SELECT address, balance_sats, total_received_sats, total_sent_sats, tx_count, utxo_count, 
		       COALESCE(first_seen_height, 0), COALESCE(last_seen_height, 0)
		FROM address_balances
		WHERE address = $1
	`, address).Scan(
		&info.Address, &info.BalanceSats, &info.TotalReceivedSats, &info.TotalSentSats,
		&info.TxCount, &info.UtxoCount, &info.FirstSeenHeight, &info.LastSeenHeight,
	)
	if err == nil {
		if err := r.setAddressStats(ctx, address, &info); err != nil {
			return nil, err
		}
		return &info, nil
	}

	if err != pgx.ErrNoRows {
		return nil, err
	}

	// Fallback: Compute from utxo_set and tx_outputs if not in cache (historical sync mode)
	info.Address = address
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(value_sats), 0), COUNT(*)
		FROM utxo_set
		WHERE address = $1
	`, address).Scan(&info.BalanceSats, &info.UtxoCount)
	if err != nil {
		return nil, err
	}

	err = r.pool.QueryRow(ctx, `
		WITH txs AS (
			SELECT txid, block_height, value_sats as received, 0 as sent
			FROM tx_outputs
			WHERE address = $1
			UNION
			SELECT spending_txid, spent_height, 0 as received, value_sats as sent
			FROM tx_outputs
			WHERE address = $1 AND is_spent = TRUE AND spending_txid IS NOT NULL
		)
		SELECT COUNT(DISTINCT txid),
		       COALESCE(MIN(block_height), 0), 
		       COALESCE(MAX(block_height), 0),
		       COALESCE(SUM(received), 0),
		       COALESCE(SUM(sent), 0)
		FROM txs
	`, address).Scan(&info.TxCount, &info.FirstSeenHeight, &info.LastSeenHeight, &info.TotalReceivedSats, &info.TotalSentSats)
	if err != nil {
		return nil, err
	}

	if info.TxCount == 0 && info.UtxoCount == 0 {
		return nil, nil
	}
	return &info, nil
}

func (r *Repository) GetAddressTransactions(ctx context.Context, address string, direction string, limit, offset int) ([]AddressTransaction, error) {
	direction = normalizeDirection(direction)
	roleFilter := ""
	switch direction {
	case "in": // Sender in user's convention
		roleFilter = "AND role = 1"
	case "out": // Receiver in user's convention
		roleFilter = "AND role = 0"
	case "both":
		roleFilter = "" // Selects both roles
	}

	query := fmt.Sprintf(`
		WITH filtered AS (
			SELECT txid, block_height, tx_index, block_time, net_value_sats, role
			FROM address_transactions
			WHERE address = $1 %s
		),
		aggregated AS (
			SELECT txid,
			       block_height,
			       MAX(tx_index) AS tx_index,
			       MAX(block_time) AS block_time,
			       SUM(net_value_sats) AS net_value_sats,
			       role
			FROM filtered
			GROUP BY txid, block_height, role
		)
		SELECT txid, block_height, block_time, net_value_sats, role
		FROM aggregated
		ORDER BY block_height DESC, tx_index DESC
		LIMIT $2 OFFSET $3
	`, roleFilter)

	rows, err := r.pool.Query(ctx, query, address, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []AddressTransaction
	hasSender := false
	for rows.Next() {
		var tx AddressTransaction
		var txid []byte
		var role int16
		err := rows.Scan(&txid, &tx.BlockHeight, &tx.BlockTime, &tx.NetValueSats, &role)
		if err != nil {
			return nil, err
		}
		tx.Txid = hex.EncodeToString(txid)
		tx.Role = formatRole(role)
		tx.Direction = formatDirection(role)
		if role == 1 || role == 2 {
			hasSender = true
		}
		txs = append(txs, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Optimization: If we found results and they satisfy the direction request, return them.
	// 'out' rows are always present in address_transactions.
	// 'in' rows are only present if backfilled.
	if len(txs) > 0 {
		if direction == "out" || hasSender {
			return txs, nil
		}
	}

	// Fallback for historical sync or missing backfill:
	// Query tx_outputs directly (slower but complete for IN/BOTH roles).
	fallbackQuery := fmt.Sprintf(`
		WITH combined AS (
			SELECT txid, block_height, value_sats as net_value, 0 as role
			FROM tx_outputs
			WHERE address = $1
			UNION ALL
			SELECT spending_txid, spent_height, -value_sats as net_value, 1 as role
			FROM tx_outputs
			WHERE address = $1 AND is_spent = TRUE
		),
		filtered AS (
			SELECT txid, block_height, net_value, role
			FROM combined
			WHERE txid IS NOT NULL %s
		),
		aggregated AS (
			SELECT txid,
			       block_height,
			       SUM(net_value) AS net_value_sats,
			       role
			FROM filtered
			GROUP BY txid, block_height, role
		)
		SELECT a.txid, a.block_height, b.block_time, a.net_value_sats, a.role
		FROM aggregated a
		LEFT JOIN blocks b ON b.height = a.block_height
		ORDER BY a.block_height DESC
		LIMIT $2 OFFSET $3
	`, roleFilter)

	rows, err = r.pool.Query(ctx, fallbackQuery, address, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	txs = txs[:0]
	for rows.Next() {
		var tx AddressTransaction
		var txid []byte
		var role int
		err := rows.Scan(&txid, &tx.BlockHeight, &tx.BlockTime, &tx.NetValueSats, &role)
		if err != nil {
			return nil, err
		}
		tx.Txid = hex.EncodeToString(txid)
		tx.Role = formatRole(int16(role))
		tx.Direction = formatDirection(int16(role))
		txs = append(txs, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return txs, nil
}

func normalizeDirection(direction string) string {
	switch strings.ToLower(direction) {
	case "in", "out", "both":
		return strings.ToLower(direction)
	default:
		return "both"
	}
}

func (r *Repository) setAddressStats(ctx context.Context, address string, info *AddressInfo) error {
	var stats AddressInfo
	err := r.pool.QueryRow(ctx, `
		WITH txs AS (
			SELECT txid, block_height, value_sats AS received, 0::BIGINT AS sent
			FROM tx_outputs
			WHERE address = $1
			UNION ALL
			SELECT spending_txid, spent_height, 0::BIGINT AS received, value_sats AS sent
			FROM tx_outputs
			WHERE address = $1
			  AND is_spent = TRUE
			  AND spending_txid IS NOT NULL
		)
		SELECT COUNT(DISTINCT txid)::INT,
		       COALESCE(MIN(block_height), 0)::INT,
		       COALESCE(MAX(block_height), 0)::INT,
		       COALESCE(SUM(received), 0)::BIGINT,
		       COALESCE(SUM(sent), 0)::BIGINT
		FROM txs
	`, address).Scan(
		&stats.TxCount,
		&stats.FirstSeenHeight,
		&stats.LastSeenHeight,
		&stats.TotalReceivedSats,
		&stats.TotalSentSats,
	)
	if err != nil {
		return err
	}
	if stats.TxCount == 0 {
		return nil
	}

	info.TxCount = stats.TxCount
	info.FirstSeenHeight = stats.FirstSeenHeight
	info.LastSeenHeight = stats.LastSeenHeight
	info.TotalReceivedSats = stats.TotalReceivedSats
	info.TotalSentSats = stats.TotalSentSats
	return nil
}

func (r *Repository) GetTransaction(ctx context.Context, txidHex string) (*TxInfo, error) {
	txid, err := hex.DecodeString(txidHex)
	if err != nil {
		return nil, fmt.Errorf("invalid txid: %w", err)
	}

	var tx TxInfo
	var txidBytes []byte
	var blockHashBytes []byte
	err = r.pool.QueryRow(ctx, `
		SELECT t.txid, t.block_height, t.block_hash, b.block_time, t.version, t.locktime, 
		       t.is_coinbase, t.fee_sats, t.size_bytes, t.vsize, t.weight
		FROM transactions t
		JOIN blocks b ON b.height = t.block_height
		WHERE t.txid = $1
		LIMIT 1
	`, txid).Scan(
		&txidBytes, &tx.BlockHeight, &blockHashBytes, &tx.BlockTime, &tx.Version, &tx.Locktime,
		&tx.IsCoinbase, &tx.FeeSats, &tx.SizeBytes, &tx.VSize, &tx.Weight,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	tx.Txid = hex.EncodeToString(txidBytes)
	tx.BlockHash = hex.EncodeToString(blockHashBytes)

	// Fetch Inputs
	inputRows, err := r.pool.Query(ctx, `
		SELECT i.vin_idx, i.prev_txid, i.prev_vout, i.script_sig, i.sequence_no,
		       o.value_sats, o.address
		FROM tx_inputs i
		LEFT JOIN tx_outputs o ON o.txid = i.prev_txid AND o.vout_idx = i.prev_vout
		WHERE i.txid = $1 AND i.block_height = $2
		ORDER BY i.vin_idx
	`, txid, tx.BlockHeight)
	if err != nil {
		return nil, err
	}
	defer inputRows.Close()

	for inputRows.Next() {
		var in TxInput
		var prevTxid []byte
		var scriptSig []byte
		var addr *string
		var val *int64
		err := inputRows.Scan(&in.Index, &prevTxid, &in.PrevVout, &scriptSig, &in.SequenceNo, &val, &addr)
		if err != nil {
			return nil, err
		}
		if prevTxid != nil {
			in.PrevTxid = hex.EncodeToString(prevTxid)
		}
		if scriptSig != nil {
			in.ScriptSig = hex.EncodeToString(scriptSig)
		}
		if addr != nil {
			in.Address = *addr
		}
		if val != nil {
			in.ValueSats = *val
		}
		tx.Inputs = append(tx.Inputs, in)
	}

	// Fetch Outputs
	outputRows, err := r.pool.Query(ctx, `
		SELECT vout_idx, value_sats, address, script_pubkey, script_type, is_spent, spending_txid
		FROM tx_outputs
		WHERE txid = $1 AND block_height = $2
		ORDER BY vout_idx
	`, txid, tx.BlockHeight)
	if err != nil {
		return nil, err
	}
	defer outputRows.Close()

	for outputRows.Next() {
		var out TxOutput
		var scriptPubKey []byte
		var scriptType int16
		var spendingTxid []byte
		var addr *string
		err := outputRows.Scan(&out.Index, &out.ValueSats, &addr, &scriptPubKey, &scriptType, &out.Spent, &spendingTxid)
		if err != nil {
			return nil, err
		}
		if addr != nil {
			out.Address = *addr
		}
		if scriptPubKey != nil {
			out.ScriptPubKey = hex.EncodeToString(scriptPubKey)
		}
		out.ScriptType = formatScriptType(scriptType)
		if spendingTxid != nil {
			out.SpendingTxid = hex.EncodeToString(spendingTxid)
		}
		tx.Outputs = append(tx.Outputs, out)
	}

	return &tx, nil
}

func (r *Repository) GetTrace(ctx context.Context, txidHex string) ([]string, error) {
	txid, err := hex.DecodeString(txidHex)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT spending_txid 
		FROM tx_outputs 
		WHERE txid = $1 AND spending_txid IS NOT NULL
	`, txid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	descendants := []string{}
	for rows.Next() {
		var dTxid []byte
		if err := rows.Scan(&dTxid); err != nil {
			return nil, err
		}
		descendants = append(descendants, hex.EncodeToString(dTxid))
	}
	return descendants, nil
}

func formatRole(role int16) string {
	switch role {
	case models.RoleReceiver:
		return "receiver"
	case models.RoleSender:
		return "sender"
	case models.RoleBoth:
		return "both"
	default:
		return "unknown"
	}
}

func formatDirection(role int16) string {
	switch role {
	case models.RoleReceiver:
		return "OUT"
	case models.RoleSender:
		return "IN"
	case models.RoleBoth:
		return "BOTH"
	default:
		return "UNKNOWN"
	}
}

func formatScriptType(st int16) string {
	switch st {
	case models.ScriptP2PKH:
		return "p2pkh"
	case models.ScriptP2SH:
		return "p2sh"
	case models.ScriptP2WPKH:
		return "p2wpkh"
	case models.ScriptP2WSH:
		return "p2wsh"
	case models.ScriptP2TR:
		return "p2tr"
	case models.ScriptOpReturn:
		return "op_return"
	case models.ScriptMultisig:
		return "multisig"
	default:
		return "unknown"
	}
}
