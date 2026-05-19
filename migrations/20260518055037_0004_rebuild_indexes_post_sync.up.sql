-- Rebuild all indexes after sync reaches tip
-- Run: migrate -path migrations -database $DB up 1
-- NOTE: CONCURRENTLY cannot run inside a transaction.
-- Run each statement separately in psql, not via migrate tool.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txout_addr_unspent
    ON tx_outputs (address, value_sats DESC)
    WHERE is_spent = FALSE AND address IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txout_spending
    ON tx_outputs (spending_txid)
    WHERE spending_txid IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txout_height_brin
    ON tx_outputs USING BRIN (block_height) WITH (pages_per_range = 32);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txin_txid
    ON tx_inputs (txid);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txin_prev
    ON tx_inputs (prev_txid, prev_vout)
    WHERE prev_txid IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_txin_height_brin
    ON tx_inputs USING BRIN (block_height) WITH (pages_per_range = 32);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tx_txid
    ON transactions (txid);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tx_coinbase
    ON transactions (block_height)
    WHERE is_coinbase = TRUE;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_tx_height_brin
    ON transactions USING BRIN (block_height) WITH (pages_per_range = 32);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_addrtx_addr
    ON address_transactions (address, block_height DESC, tx_index DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_addrtx_height_brin
    ON address_transactions USING BRIN (block_height) WITH (pages_per_range = 32);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_utxo_address
    ON utxo_set (address, value_sats DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_utxo_height_brin
    ON utxo_set USING BRIN (block_height) WITH (pages_per_range = 32);

-- Re-enable autovacuum
ALTER SYSTEM SET autovacuum = on;
SELECT pg_reload_conf();

ALTER TABLE utxo_set SET (autovacuum_vacuum_scale_factor = 0.005, autovacuum_vacuum_cost_limit = 1000);
ALTER TABLE address_balances SET (autovacuum_vacuum_scale_factor = 0.005);
ALTER TABLE address_transactions SET (autovacuum_vacuum_scale_factor = 0.01);

ANALYZE blocks;
ANALYZE transactions;
ANALYZE tx_outputs;
ANALYZE tx_inputs;
ANALYZE utxo_set;
ANALYZE address_balances;