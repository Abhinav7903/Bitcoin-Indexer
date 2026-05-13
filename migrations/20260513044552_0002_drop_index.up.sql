-- ============================================================
-- UP: Drop heavy indexes for maximum backfill speed
-- 
-- WHY: These indexes are maintained on every INSERT/UPDATE.
-- During historical backfill they cost minutes per batch but
-- provide zero benefit (no queries run against them).
-- They are rebuilt by the DOWN migration after backfill.
--
-- WHAT WE KEEP (required for correctness during backfill):
--   - tx_outputs PRIMARY KEY (block_height, txid, vout_idx)
--   - idx_txout_txid (txid, vout_idx)  ← applySpendState join
--   - idx_txout_height_brin            ← partition pruning
--   - transactions PRIMARY KEY         ← sender backfill join
--   - tx_inputs PRIMARY KEY            ← correctness
--   - blocks PRIMARY KEY               ← correctness
--   - utxo_set PRIMARY KEY             ← correctness
--   - index_state PRIMARY KEY          ← checkpoint
--
-- WHAT WE DROP (not needed during backfill):
--   - idx_txout_addr_unspent  (UTXO address lookups)
--   - idx_txout_spending      (spend lookups, replaced by temp table)
--   - idx_txin_txid           (input lookups)
--   - idx_txin_prev           (input->output joins, not needed during load)
--   - idx_tx_txid             (tx lookups by txid only)
--   - idx_addrtx_addr         (address history queries)
--   - idx_utxo_address        (UTXO address queries)
-- ============================================================

-- tx_outputs: biggest win — 14 GB of indexes on the hot partition
DROP INDEX IF EXISTS idx_txout_addr_unspent;
DROP INDEX IF EXISTS idx_txout_spending;

-- tx_inputs: saves ~2-3 GB per 100k block range
DROP INDEX IF EXISTS idx_txin_txid;
DROP INDEX IF EXISTS idx_txin_prev;

-- transactions: saves ~1-2 GB per 100k block range
DROP INDEX IF EXISTS idx_tx_txid;

-- address_transactions: not written during historical_sync=true
-- but drop anyway to keep future partitions lean
DROP INDEX IF EXISTS idx_addrtx_addr;

-- utxo_set: address lookup index, not needed during backfill
DROP INDEX IF EXISTS idx_utxo_address;