-- ============================================================
-- 0004_rebuild_indexes_post_sync.up.sql (FIXED for PG16)
-- NO CONCURRENTLY on partitioned tables
-- autovacuum SET on leaf partitions only
-- ============================================================

-- ============================================================
-- tx_outputs
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_0_100k    ON tx_outputs_0_100k    (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_100k_200k ON tx_outputs_100k_200k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_200k_300k ON tx_outputs_200k_300k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_300k_400k ON tx_outputs_300k_400k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_400k_500k ON tx_outputs_400k_500k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_500k_600k ON tx_outputs_500k_600k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_600k_700k ON tx_outputs_600k_700k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_700k_800k ON tx_outputs_700k_800k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_800k_900k ON tx_outputs_800k_900k (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_900k_1m   ON tx_outputs_900k_1m   (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_1m_11m    ON tx_outputs_1m_11m    (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_addr_unspent_default   ON tx_outputs_default   (address, value_sats DESC) WHERE is_spent = FALSE AND address IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_txout_spending_0_100k    ON tx_outputs_0_100k    (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_100k_200k ON tx_outputs_100k_200k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_200k_300k ON tx_outputs_200k_300k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_300k_400k ON tx_outputs_300k_400k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_400k_500k ON tx_outputs_400k_500k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_500k_600k ON tx_outputs_500k_600k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_600k_700k ON tx_outputs_600k_700k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_700k_800k ON tx_outputs_700k_800k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_800k_900k ON tx_outputs_800k_900k (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_900k_1m   ON tx_outputs_900k_1m   (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_1m_11m    ON tx_outputs_1m_11m    (spending_txid) WHERE spending_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txout_spending_default   ON tx_outputs_default   (spending_txid) WHERE spending_txid IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_txout_brin_0_100k    ON tx_outputs_0_100k    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_100k_200k ON tx_outputs_100k_200k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_200k_300k ON tx_outputs_200k_300k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_300k_400k ON tx_outputs_300k_400k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_400k_500k ON tx_outputs_400k_500k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_500k_600k ON tx_outputs_500k_600k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_600k_700k ON tx_outputs_600k_700k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_700k_800k ON tx_outputs_700k_800k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_800k_900k ON tx_outputs_800k_900k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_900k_1m   ON tx_outputs_900k_1m   USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_1m_11m    ON tx_outputs_1m_11m    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txout_brin_default   ON tx_outputs_default   USING BRIN (block_height) WITH (pages_per_range=32);

-- ============================================================
-- tx_inputs
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_txin_txid_0_100k    ON tx_inputs_0_100k    (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_100k_200k ON tx_inputs_100k_200k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_200k_300k ON tx_inputs_200k_300k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_300k_400k ON tx_inputs_300k_400k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_400k_500k ON tx_inputs_400k_500k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_500k_600k ON tx_inputs_500k_600k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_600k_700k ON tx_inputs_600k_700k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_700k_800k ON tx_inputs_700k_800k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_800k_900k ON tx_inputs_800k_900k (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_900k_1m   ON tx_inputs_900k_1m   (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_1m_11m    ON tx_inputs_1m_11m    (txid);
CREATE INDEX IF NOT EXISTS idx_txin_txid_default   ON tx_inputs_default   (txid);

CREATE INDEX IF NOT EXISTS idx_txin_prev_0_100k    ON tx_inputs_0_100k    (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_100k_200k ON tx_inputs_100k_200k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_200k_300k ON tx_inputs_200k_300k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_300k_400k ON tx_inputs_300k_400k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_400k_500k ON tx_inputs_400k_500k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_500k_600k ON tx_inputs_500k_600k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_600k_700k ON tx_inputs_600k_700k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_700k_800k ON tx_inputs_700k_800k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_800k_900k ON tx_inputs_800k_900k (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_900k_1m   ON tx_inputs_900k_1m   (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_1m_11m    ON tx_inputs_1m_11m    (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_txin_prev_default   ON tx_inputs_default   (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_txin_brin_0_100k    ON tx_inputs_0_100k    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_100k_200k ON tx_inputs_100k_200k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_200k_300k ON tx_inputs_200k_300k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_300k_400k ON tx_inputs_300k_400k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_400k_500k ON tx_inputs_400k_500k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_500k_600k ON tx_inputs_500k_600k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_600k_700k ON tx_inputs_600k_700k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_700k_800k ON tx_inputs_700k_800k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_800k_900k ON tx_inputs_800k_900k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_900k_1m   ON tx_inputs_900k_1m   USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_1m_11m    ON tx_inputs_1m_11m    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_txin_brin_default   ON tx_inputs_default   USING BRIN (block_height) WITH (pages_per_range=32);

-- ============================================================
-- transactions
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_tx_txid_0_100k    ON transactions_0_100k    (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_100k_200k ON transactions_100k_200k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_200k_300k ON transactions_200k_300k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_300k_400k ON transactions_300k_400k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_400k_500k ON transactions_400k_500k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_500k_600k ON transactions_500k_600k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_600k_700k ON transactions_600k_700k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_700k_800k ON transactions_700k_800k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_800k_900k ON transactions_800k_900k (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_900k_1m   ON transactions_900k_1m   (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_1m_11m    ON transactions_1m_11m    (txid);
CREATE INDEX IF NOT EXISTS idx_tx_txid_default   ON transactions_default   (txid);

CREATE INDEX IF NOT EXISTS idx_tx_coinbase_0_100k    ON transactions_0_100k    (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_100k_200k ON transactions_100k_200k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_200k_300k ON transactions_200k_300k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_300k_400k ON transactions_300k_400k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_400k_500k ON transactions_400k_500k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_500k_600k ON transactions_500k_600k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_600k_700k ON transactions_600k_700k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_700k_800k ON transactions_700k_800k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_800k_900k ON transactions_800k_900k (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_900k_1m   ON transactions_900k_1m   (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_1m_11m    ON transactions_1m_11m    (block_height) WHERE is_coinbase = TRUE;
CREATE INDEX IF NOT EXISTS idx_tx_coinbase_default   ON transactions_default   (block_height) WHERE is_coinbase = TRUE;

CREATE INDEX IF NOT EXISTS idx_tx_brin_0_100k    ON transactions_0_100k    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_100k_200k ON transactions_100k_200k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_200k_300k ON transactions_200k_300k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_300k_400k ON transactions_300k_400k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_400k_500k ON transactions_400k_500k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_500k_600k ON transactions_500k_600k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_600k_700k ON transactions_600k_700k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_700k_800k ON transactions_700k_800k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_800k_900k ON transactions_800k_900k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_900k_1m   ON transactions_900k_1m   USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_1m_11m    ON transactions_1m_11m    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_tx_brin_default   ON transactions_default   USING BRIN (block_height) WITH (pages_per_range=32);

-- ============================================================
-- address_transactions
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_0_100k    ON address_tx_0_100k    (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_100k_200k ON address_tx_100k_200k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_200k_300k ON address_tx_200k_300k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_300k_400k ON address_tx_300k_400k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_400k_500k ON address_tx_400k_500k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_500k_600k ON address_tx_500k_600k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_600k_700k ON address_tx_600k_700k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_700k_800k ON address_tx_700k_800k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_800k_900k ON address_tx_800k_900k (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_900k_1m   ON address_tx_900k_1m   (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_1m_11m    ON address_tx_1m_11m    (address, block_height DESC, tx_index DESC);
CREATE INDEX IF NOT EXISTS idx_addrtx_addr_default   ON address_tx_default   (address, block_height DESC, tx_index DESC);

CREATE INDEX IF NOT EXISTS idx_addrtx_brin_0_100k    ON address_tx_0_100k    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_100k_200k ON address_tx_100k_200k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_200k_300k ON address_tx_200k_300k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_300k_400k ON address_tx_300k_400k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_400k_500k ON address_tx_400k_500k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_500k_600k ON address_tx_500k_600k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_600k_700k ON address_tx_600k_700k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_700k_800k ON address_tx_700k_800k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_800k_900k ON address_tx_800k_900k USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_900k_1m   ON address_tx_900k_1m   USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_1m_11m    ON address_tx_1m_11m    USING BRIN (block_height) WITH (pages_per_range=32);
CREATE INDEX IF NOT EXISTS idx_addrtx_brin_default   ON address_tx_default   USING BRIN (block_height) WITH (pages_per_range=32);

-- ============================================================
-- utxo_set (not partitioned — straightforward)
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_utxo_address    ON utxo_set (address, value_sats DESC);
CREATE INDEX IF NOT EXISTS idx_utxo_height_brin ON utxo_set USING BRIN (block_height) WITH (pages_per_range=32);

-- ============================================================
-- Re-enable autovacuum
-- FIX: SET on leaf partitions only (not parent partitioned table)
-- ============================================================
ALTER SYSTEM SET autovacuum = on;
SELECT pg_reload_conf();

-- utxo_set is not partitioned — SET directly
ALTER TABLE utxo_set SET (
    autovacuum_vacuum_scale_factor = 0.005,
    autovacuum_vacuum_cost_limit   = 1000
);

-- address_balances is not partitioned — SET directly
ALTER TABLE address_balances SET (
    autovacuum_vacuum_scale_factor = 0.005
);

-- address_transactions is partitioned — SET on each leaf partition
ALTER TABLE address_tx_0_100k    SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_100k_200k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_200k_300k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_300k_400k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_400k_500k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_500k_600k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_600k_700k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_700k_800k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_800k_900k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_900k_1m   SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_1m_11m    SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE address_tx_default   SET (autovacuum_vacuum_scale_factor = 0.01);

-- tx_outputs leaf partitions
ALTER TABLE tx_outputs_0_100k    SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_100k_200k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_200k_300k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_300k_400k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_400k_500k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_500k_600k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_600k_700k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_700k_800k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_800k_900k SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_900k_1m   SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_1m_11m    SET (autovacuum_vacuum_scale_factor = 0.01);
ALTER TABLE tx_outputs_default   SET (autovacuum_vacuum_scale_factor = 0.01);

-- ============================================================
-- ANALYZE
-- ============================================================
ANALYZE blocks;
ANALYZE transactions;
ANALYZE tx_outputs;
ANALYZE tx_inputs;
ANALYZE address_transactions;
ANALYZE utxo_set;
ANALYZE address_balances;


