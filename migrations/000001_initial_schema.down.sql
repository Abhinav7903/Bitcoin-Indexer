-- Original order was wrong — btree_gin was dropped before age CASCADE

DROP TABLE IF EXISTS index_state;
DROP TABLE IF EXISTS address_balances;
DROP TABLE IF EXISTS utxo_set;
DROP TABLE IF EXISTS address_transactions;
DROP TABLE IF EXISTS tx_inputs;
DROP TABLE IF EXISTS tx_outputs;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS blocks;

DROP GRAPH IF EXISTS bitcoin CASCADE;

-- FIX: Drop age CASCADE first, then the other extensions
DROP EXTENSION IF EXISTS age CASCADE;
DROP EXTENSION IF EXISTS btree_gin;
DROP EXTENSION IF EXISTS pg_trgm;