-- ============================================================
-- 0004_rebuild_indexes_post_sync.down.sql
-- Drops all indexes created by 0004 up
-- Use if you need to re-run bulk operations again
-- ============================================================

-- ============================================================
-- tx_outputs
-- ============================================================
DROP INDEX IF EXISTS idx_txout_addr_unspent_0_100k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_100k_200k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_200k_300k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_300k_400k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_400k_500k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_500k_600k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_600k_700k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_700k_800k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_800k_900k;
DROP INDEX IF EXISTS idx_txout_addr_unspent_900k_1m;
DROP INDEX IF EXISTS idx_txout_addr_unspent_1m_11m;
DROP INDEX IF EXISTS idx_txout_addr_unspent_default;

DROP INDEX IF EXISTS idx_txout_spending_0_100k;
DROP INDEX IF EXISTS idx_txout_spending_100k_200k;
DROP INDEX IF EXISTS idx_txout_spending_200k_300k;
DROP INDEX IF EXISTS idx_txout_spending_300k_400k;
DROP INDEX IF EXISTS idx_txout_spending_400k_500k;
DROP INDEX IF EXISTS idx_txout_spending_500k_600k;
DROP INDEX IF EXISTS idx_txout_spending_600k_700k;
DROP INDEX IF EXISTS idx_txout_spending_700k_800k;
DROP INDEX IF EXISTS idx_txout_spending_800k_900k;
DROP INDEX IF EXISTS idx_txout_spending_900k_1m;
DROP INDEX IF EXISTS idx_txout_spending_1m_11m;
DROP INDEX IF EXISTS idx_txout_spending_default;

DROP INDEX IF EXISTS idx_txout_brin_0_100k;
DROP INDEX IF EXISTS idx_txout_brin_100k_200k;
DROP INDEX IF EXISTS idx_txout_brin_200k_300k;
DROP INDEX IF EXISTS idx_txout_brin_300k_400k;
DROP INDEX IF EXISTS idx_txout_brin_400k_500k;
DROP INDEX IF EXISTS idx_txout_brin_500k_600k;
DROP INDEX IF EXISTS idx_txout_brin_600k_700k;
DROP INDEX IF EXISTS idx_txout_brin_700k_800k;
DROP INDEX IF EXISTS idx_txout_brin_800k_900k;
DROP INDEX IF EXISTS idx_txout_brin_900k_1m;
DROP INDEX IF EXISTS idx_txout_brin_1m_11m;
DROP INDEX IF EXISTS idx_txout_brin_default;

-- ============================================================
-- tx_inputs
-- ============================================================
DROP INDEX IF EXISTS idx_txin_txid_0_100k;
DROP INDEX IF EXISTS idx_txin_txid_100k_200k;
DROP INDEX IF EXISTS idx_txin_txid_200k_300k;
DROP INDEX IF EXISTS idx_txin_txid_300k_400k;
DROP INDEX IF EXISTS idx_txin_txid_400k_500k;
DROP INDEX IF EXISTS idx_txin_txid_500k_600k;
DROP INDEX IF EXISTS idx_txin_txid_600k_700k;
DROP INDEX IF EXISTS idx_txin_txid_700k_800k;
DROP INDEX IF EXISTS idx_txin_txid_800k_900k;
DROP INDEX IF EXISTS idx_txin_txid_900k_1m;
DROP INDEX IF EXISTS idx_txin_txid_1m_11m;
DROP INDEX IF EXISTS idx_txin_txid_default;

DROP INDEX IF EXISTS idx_txin_prev_0_100k;
DROP INDEX IF EXISTS idx_txin_prev_100k_200k;
DROP INDEX IF EXISTS idx_txin_prev_200k_300k;
DROP INDEX IF EXISTS idx_txin_prev_300k_400k;
DROP INDEX IF EXISTS idx_txin_prev_400k_500k;
DROP INDEX IF EXISTS idx_txin_prev_500k_600k;
DROP INDEX IF EXISTS idx_txin_prev_600k_700k;
DROP INDEX IF EXISTS idx_txin_prev_700k_800k;
DROP INDEX IF EXISTS idx_txin_prev_800k_900k;
DROP INDEX IF EXISTS idx_txin_prev_900k_1m;
DROP INDEX IF EXISTS idx_txin_prev_1m_11m;
DROP INDEX IF EXISTS idx_txin_prev_default;

DROP INDEX IF EXISTS idx_txin_brin_0_100k;
DROP INDEX IF EXISTS idx_txin_brin_100k_200k;
DROP INDEX IF EXISTS idx_txin_brin_200k_300k;
DROP INDEX IF EXISTS idx_txin_brin_300k_400k;
DROP INDEX IF EXISTS idx_txin_brin_400k_500k;
DROP INDEX IF EXISTS idx_txin_brin_500k_600k;
DROP INDEX IF EXISTS idx_txin_brin_600k_700k;
DROP INDEX IF EXISTS idx_txin_brin_700k_800k;
DROP INDEX IF EXISTS idx_txin_brin_800k_900k;
DROP INDEX IF EXISTS idx_txin_brin_900k_1m;
DROP INDEX IF EXISTS idx_txin_brin_1m_11m;
DROP INDEX IF EXISTS idx_txin_brin_default;

-- ============================================================
-- transactions
-- ============================================================
DROP INDEX IF EXISTS idx_tx_txid_0_100k;
DROP INDEX IF EXISTS idx_tx_txid_100k_200k;
DROP INDEX IF EXISTS idx_tx_txid_200k_300k;
DROP INDEX IF EXISTS idx_tx_txid_300k_400k;
DROP INDEX IF EXISTS idx_tx_txid_400k_500k;
DROP INDEX IF EXISTS idx_tx_txid_500k_600k;
DROP INDEX IF EXISTS idx_tx_txid_600k_700k;
DROP INDEX IF EXISTS idx_tx_txid_700k_800k;
DROP INDEX IF EXISTS idx_tx_txid_800k_900k;
DROP INDEX IF EXISTS idx_tx_txid_900k_1m;
DROP INDEX IF EXISTS idx_tx_txid_1m_11m;
DROP INDEX IF EXISTS idx_tx_txid_default;

DROP INDEX IF EXISTS idx_tx_coinbase_0_100k;
DROP INDEX IF EXISTS idx_tx_coinbase_100k_200k;
DROP INDEX IF EXISTS idx_tx_coinbase_200k_300k;
DROP INDEX IF EXISTS idx_tx_coinbase_300k_400k;
DROP INDEX IF EXISTS idx_tx_coinbase_400k_500k;
DROP INDEX IF EXISTS idx_tx_coinbase_500k_600k;
DROP INDEX IF EXISTS idx_tx_coinbase_600k_700k;
DROP INDEX IF EXISTS idx_tx_coinbase_700k_800k;
DROP INDEX IF EXISTS idx_tx_coinbase_800k_900k;
DROP INDEX IF EXISTS idx_tx_coinbase_900k_1m;
DROP INDEX IF EXISTS idx_tx_coinbase_1m_11m;
DROP INDEX IF EXISTS idx_tx_coinbase_default;

DROP INDEX IF EXISTS idx_tx_brin_0_100k;
DROP INDEX IF EXISTS idx_tx_brin_100k_200k;
DROP INDEX IF EXISTS idx_tx_brin_200k_300k;
DROP INDEX IF EXISTS idx_tx_brin_300k_400k;
DROP INDEX IF EXISTS idx_tx_brin_400k_500k;
DROP INDEX IF EXISTS idx_tx_brin_500k_600k;
DROP INDEX IF EXISTS idx_tx_brin_600k_700k;
DROP INDEX IF EXISTS idx_tx_brin_700k_800k;
DROP INDEX IF EXISTS idx_tx_brin_800k_900k;
DROP INDEX IF EXISTS idx_tx_brin_900k_1m;
DROP INDEX IF EXISTS idx_tx_brin_1m_11m;
DROP INDEX IF EXISTS idx_tx_brin_default;

-- ============================================================
-- address_transactions
-- ============================================================
DROP INDEX IF EXISTS idx_addrtx_addr_0_100k;
DROP INDEX IF EXISTS idx_addrtx_addr_100k_200k;
DROP INDEX IF EXISTS idx_addrtx_addr_200k_300k;
DROP INDEX IF EXISTS idx_addrtx_addr_300k_400k;
DROP INDEX IF EXISTS idx_addrtx_addr_400k_500k;
DROP INDEX IF EXISTS idx_addrtx_addr_500k_600k;
DROP INDEX IF EXISTS idx_addrtx_addr_600k_700k;
DROP INDEX IF EXISTS idx_addrtx_addr_700k_800k;
DROP INDEX IF EXISTS idx_addrtx_addr_800k_900k;
DROP INDEX IF EXISTS idx_addrtx_addr_900k_1m;
DROP INDEX IF EXISTS idx_addrtx_addr_1m_11m;
DROP INDEX IF EXISTS idx_addrtx_addr_default;

DROP INDEX IF EXISTS idx_addrtx_brin_0_100k;
DROP INDEX IF EXISTS idx_addrtx_brin_100k_200k;
DROP INDEX IF EXISTS idx_addrtx_brin_200k_300k;
DROP INDEX IF EXISTS idx_addrtx_brin_300k_400k;
DROP INDEX IF EXISTS idx_addrtx_brin_400k_500k;
DROP INDEX IF EXISTS idx_addrtx_brin_500k_600k;
DROP INDEX IF EXISTS idx_addrtx_brin_600k_700k;
DROP INDEX IF EXISTS idx_addrtx_brin_700k_800k;
DROP INDEX IF EXISTS idx_addrtx_brin_800k_900k;
DROP INDEX IF EXISTS idx_addrtx_brin_900k_1m;
DROP INDEX IF EXISTS idx_addrtx_brin_1m_11m;
DROP INDEX IF EXISTS idx_addrtx_brin_default;

-- ============================================================
-- utxo_set
-- ============================================================
DROP INDEX IF EXISTS idx_utxo_address;
DROP INDEX IF EXISTS idx_utxo_height_brin;

-- ============================================================
-- Reset autovacuum back to off (sync mode)
-- ============================================================
ALTER SYSTEM SET autovacuum = off;
SELECT pg_reload_conf();

\echo 'Down complete — all indexes dropped, autovacuum off'