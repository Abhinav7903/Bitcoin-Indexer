-- ============================================================
-- DOWN: Restore all indexes dropped for backfill speed
-- Run this AFTER backfill is complete and indexer is at tip
-- Use CONCURRENTLY to avoid locking during live operation
-- ============================================================

-- tx_outputs: restore heavy indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txout_addr_unspent
    ON tx_outputs (address, value_sats DESC)
    WHERE is_spent = FALSE AND address IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txout_spending
    ON tx_outputs (spending_txid)
    WHERE spending_txid IS NOT NULL;

-- tx_inputs: restore indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txin_txid
    ON tx_inputs (txid);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txin_prev
    ON tx_inputs (prev_txid, prev_vout)
    WHERE prev_txid IS NOT NULL;

-- transactions: restore txid index
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tx_txid
    ON transactions (txid);

-- address_transactions: restore indexes
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_addrtx_addr
    ON address_transactions (address, block_height DESC, tx_index DESC);

-- utxo_set: restore address index
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_utxo_address
    ON utxo_set (address, value_sats DESC);