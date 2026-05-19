-- Drop heavy indexes for backfill speed
-- Run: migrate -path migrations -database $DB up 1

DROP INDEX IF EXISTS idx_txout_addr_unspent;
DROP INDEX IF EXISTS idx_txout_spending;
DROP INDEX IF EXISTS idx_txin_txid;
DROP INDEX IF EXISTS idx_txin_prev;
DROP INDEX IF EXISTS idx_tx_txid;
DROP INDEX IF EXISTS idx_tx_coinbase;
DROP INDEX IF EXISTS idx_addrtx_addr;
DROP INDEX IF EXISTS idx_addrtx_height_brin;
DROP INDEX IF EXISTS idx_utxo_address;
DROP INDEX IF EXISTS idx_utxo_height_brin;
DROP INDEX IF EXISTS idx_tx_height_brin;
DROP INDEX IF EXISTS idx_txout_height_brin;
DROP INDEX IF EXISTS idx_txin_height_brin;