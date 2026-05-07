package api

import (
	"context"
	"encoding/hex"
	"fmt"

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
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &info, nil
}

func (r *Repository) GetAddressTransactions(ctx context.Context, address string, limit, offset int) ([]AddressTransaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT txid, block_height, block_time, net_value_sats, role
		FROM address_transactions
		WHERE address = $1
		ORDER BY block_height DESC, tx_index DESC
		LIMIT $2 OFFSET $3
	`, address, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []AddressTransaction
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
		txs = append(txs, tx)
	}
	return txs, nil
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
		err := outputRows.Scan(&out.Index, &out.ValueSats, &out.Address, &scriptPubKey, &scriptType, &out.Spent, &spendingTxid)
		if err != nil {
			return nil, err
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

	var descendants []string
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
