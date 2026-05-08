# Database Schema

> PostgreSQL 16 schema reference for Bitcoin Indexer — tables, columns, partitioning, and indexes.

---

## Overview

Bitcoin Indexer maintains seven core tables in PostgreSQL. Large historical tables are **partitioned by block height** for query performance and manageability. Lookup tables are denormalized to avoid expensive joins at query time.

```
PostgreSQL 16 (btcindex)
│
├── blocks                    ← one row per block
├── transactions              ← one row per transaction (partitioned)
├── tx_inputs                 ← one row per transaction input (partitioned)
├── tx_outputs                ← one row per transaction output (partitioned)
├── address_transactions      ← denormalized address → tx mapping (partitioned)
├── utxo_set                  ← current unspent outputs
└── address_balances          ← current balance per address
```

---

## Entity Relationship Overview

```
blocks
  │
  └── transactions (block_height FK)
        │
        ├── tx_inputs  (txid FK)
        │     └── references tx_outputs (prev_txid, prev_vout)
        │
        └── tx_outputs (txid FK)
              │
              ├── address_transactions (address, txid)
              ├── utxo_set (txid, vout_idx)
              └── address_balances (address)
```

---

## Table Reference

### `blocks`

One row per Bitcoin block.

| Column | Type | Description |
|---|---|---|
| `height` | `int` PK | Block height (0-indexed) |
| `hash` | `bytea` UNIQUE | Block hash |
| `prev_hash` | `bytea` | Previous block hash |
| `merkle_root` | `bytea` | Merkle root of transactions |
| `block_time` | `timestamptz` | Block timestamp |
| `bits` | `bigint` | Difficulty target (encoded) |
| `nonce` | `bigint` | Proof-of-work nonce |
| `version` | `int` | Block version |
| `tx_count` | `int` | Number of transactions |
| `size_bytes` | `int` | Block size in bytes |
| `weight` | `int` | Block weight units |
| `total_fees_sats` | `bigint` | Sum of all transaction fees in this block |
| `indexed_at` | `timestamptz` | When this row was written |

---

### `transactions`

One row per transaction. **Partitioned by `block_height`.**

| Column | Type | Description |
|---|---|---|
| `txid` | `bytea` | Transaction ID (hash) |
| `block_height` | `int` | Block height (partition key, part of PK) |
| `block_hash` | `bytea` | Parent block hash |
| `tx_index` | `int` | Position within block (0-indexed) |
| `version` | `int` | Transaction version |
| `locktime` | `bigint` | Transaction locktime |
| `is_coinbase` | `bool` | True if coinbase transaction |
| `input_count` | `smallint` | Number of inputs |
| `output_count` | `smallint` | Number of outputs |
| `fee_sats` | `bigint` | Fee in satoshis (null for coinbase) |
| `size_bytes` | `int` | Size in bytes |
| `vsize` | `int` | Virtual size (SegWit weight / 4) |
| `weight` | `int` | Weight units |
| `has_segwit` | `bool` | True if transaction uses SegWit |

Primary key: `(block_height, txid)`

---

### `tx_inputs`

One row per transaction input. **Partitioned by `block_height`.**

| Column | Type | Description |
|---|---|---|
| `txid` | `bytea` | Parent transaction ID |
| `vin_idx` | `int` | Input index within transaction |
| `block_height` | `int` | Block height (partition key, part of PK) |
| `prev_txid` | `bytea` | Previous output's transaction ID (null for coinbase) |
| `prev_vout` | `int` | Previous output's index (null for coinbase) |
| `script_sig` | `bytea` | Input script |
| `witness_data` | `bytea[]` | SegWit witness data |
| `sequence_no` | `bigint` | Sequence number (default `0xFFFFFFFE`) |

Primary key: `(block_height, txid, vin_idx)`

---

### `tx_outputs`

One row per transaction output. **Partitioned by `block_height`.**

| Column | Type | Description |
|---|---|---|
| `txid` | `bytea` | Parent transaction ID |
| `vout_idx` | `int` | Output index within transaction |
| `block_height` | `int` | Block height (partition key, part of PK) |
| `value_sats` | `bigint` | Value in satoshis |
| `script_pubkey` | `bytea` | Output script |
| `script_type` | `smallint` | Script type enum (default 7 = unknown) |
| `address` | `text` | Decoded address (null if undecodable) |
| `is_spent` | `bool` | True if this output has been spent |
| `spending_txid` | `bytea` | TXID of the transaction that spent this output |
| `spending_vin` | `int` | Input index in the spending transaction |
| `spent_height` | `int` | Block height at which this output was spent |

Primary key: `(block_height, txid, vout_idx)`

---

### `address_transactions`

Denormalized mapping of Bitcoin addresses to transactions. Used for fast address history queries without joining `transactions`, `tx_inputs`, and `tx_outputs`. **Partitioned by `block_height`.**

| Column | Type | Description |
|---|---|---|
| `address` | `text` | Bitcoin address |
| `block_height` | `int` | Block height (partition key, part of PK) |
| `tx_index` | `int` | Position of the transaction within its block |
| `txid` | `bytea` | Transaction ID |
| `role` | `smallint` | `0` = receiver (output), `1` = sender (input) |
| `net_value_sats` | `bigint` | Net value change for this address in this tx |
| `block_time` | `timestamptz` | Block timestamp |

Primary key: `(address, block_height, tx_index, txid, role)`

---

### `utxo_set`

Current unspent transaction output set. Outputs are inserted when created and deleted when spent.

| Column | Type | Description |
|---|---|---|
| `txid` | `bytea` | Transaction ID |
| `vout_idx` | `int` | Output index |
| `address` | `text` | Output address |
| `value_sats` | `bigint` | Value in satoshis |
| `block_height` | `int` | Block height where output was created |
| `script_type` | `smallint` | Script type enum (default 7 = unknown) |

Primary key: `(txid, vout_idx)`

---

### `address_balances`

Denormalized current balance per Bitcoin address. Updated incrementally as blocks are indexed.

| Column | Type | Description |
|---|---|---|
| `address` | `text` PK | Bitcoin address |
| `balance_sats` | `bigint` | Current balance in satoshis |
| `total_received_sats` | `bigint` | Cumulative satoshis received |
| `total_sent_sats` | `bigint` | Cumulative satoshis sent |
| `utxo_count` | `int` | Number of unspent outputs held |
| `tx_count` | `int` | Total number of transactions |
| `first_seen_height` | `int` | Block height of first appearance |
| `last_seen_height` | `int` | Block height of most recent transaction |
| `updated_at_height` | `int` | Block height at which this row was last updated |

---

### `index_state`

Single-row checkpoint tracking the indexer's progress.

| Column | Type | Description |
|---|---|---|
| `id` | `int` PK | Always `1` (enforced by check constraint) |
| `last_indexed_height` | `int` | Most recently fully indexed block height |
| `last_indexed_hash` | `bytea` | Hash of the most recently indexed block |
| `started_at` | `timestamptz` | When the indexer was first started |
| `updated_at` | `timestamptz` | When this row was last updated |

---

## Partitioning

Large historical tables are range-partitioned by `block_height` in ranges of 100,000 blocks. A `DEFAULT` partition catches any rows outside defined ranges.

### Partition Strategy

```sql
-- Tables are partitioned in ranges of 100,000 blocks
transactions_0_100k
transactions_100k_200k
transactions_200k_300k
...
transactions_1m_11m
transactions_default   -- catch-all for out-of-range rows
```

The same partition naming scheme applies to `tx_outputs`, `tx_inputs`, and `address_transactions` (prefixed `tx_outputs_`, `tx_inputs_`, `address_tx_`).

Partitioning provides:
- Faster range queries by block height
- Easier data management (drop old partitions)
- Better vacuum and analyze performance
- Reduced index size per partition

### Checking Partitions

```sql
-- List all transaction partitions
SELECT tablename
FROM pg_tables
WHERE tablename LIKE 'transactions_%'
ORDER BY tablename;

-- Check row counts per partition
SELECT
  child.relname AS partition,
  pg_stat_user_tables.n_live_tup AS rows
FROM pg_inherits
JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
JOIN pg_class child ON pg_inherits.inhrelid = child.oid
JOIN pg_stat_user_tables ON pg_stat_user_tables.relname = child.relname
WHERE parent.relname = 'transactions'
ORDER BY child.relname;
```

---

## Indexes

### `blocks`
| Index | Type | Columns |
|---|---|---|
| `idx_blocks_hash` | B-tree | `hash` |
| `idx_blocks_height_brin` | BRIN | `height` |
| `idx_blocks_time_brin` | BRIN | `block_time` |

### `transactions`
| Index | Type | Columns / Condition |
|---|---|---|
| `idx_tx_txid` | B-tree | `txid` |
| `idx_tx_height_brin` | BRIN | `block_height` |
| `idx_tx_coinbase` | B-tree | `block_height` WHERE `is_coinbase = TRUE` |

### `tx_outputs`
| Index | Type | Columns / Condition |
|---|---|---|
| `idx_txout_txid` | B-tree | `txid, vout_idx` |
| `idx_txout_height_brin` | BRIN | `block_height` |
| `idx_txout_addr_unspent` | B-tree | `address, value_sats DESC` WHERE `is_spent = FALSE AND address IS NOT NULL` |
| `idx_txout_spending` | B-tree | `spending_txid` WHERE `spending_txid IS NOT NULL` |

### `tx_inputs`
| Index | Type | Columns / Condition |
|---|---|---|
| `idx_txin_txid` | B-tree | `txid` |
| `idx_txin_prev` | B-tree | `prev_txid, prev_vout` WHERE `prev_txid IS NOT NULL` |
| `idx_txin_height_brin` | BRIN | `block_height` |

### `address_transactions`
| Index | Type | Columns |
|---|---|---|
| `idx_addrtx_addr` | B-tree | `address, block_height DESC, tx_index DESC` |
| `idx_addrtx_height_brin` | BRIN | `block_height` |

### `utxo_set`
| Index | Type | Columns |
|---|---|---|
| `idx_utxo_address` | B-tree | `address, value_sats DESC` |
| `idx_utxo_height_brin` | BRIN | `block_height` |

---

## Useful Queries

### Current indexed height

```sql
SELECT last_indexed_height FROM index_state WHERE id = 1;
-- or:
SELECT MAX(height) AS indexed_height FROM blocks;
```

### Block summary

```sql
SELECT height, hash, tx_count, block_time
FROM blocks
ORDER BY height DESC
LIMIT 10;
```

### Address transaction history

```sql
SELECT txid, block_height, block_time, net_value_sats, role
FROM address_transactions
WHERE address = 'bc1qxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
ORDER BY block_height DESC, tx_index DESC
LIMIT 50;
```

### Address balance

```sql
SELECT address, balance_sats, total_received_sats, total_sent_sats, utxo_count, tx_count, last_seen_height
FROM address_balances
WHERE address = 'bc1qxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx';
```

### UTXO set for an address

```sql
SELECT txid, vout_idx, value_sats, block_height
FROM utxo_set
WHERE address = 'bc1qxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
ORDER BY block_height;
```

### Largest transactions by output value

```sql
SELECT t.txid, t.block_height, SUM(o.value_sats) AS total_output_sats
FROM transactions t
JOIN tx_outputs o ON o.txid = t.txid AND o.block_height = t.block_height
GROUP BY t.txid, t.block_height
ORDER BY total_output_sats DESC
LIMIT 20;
```

### Find unspent outputs for a transaction

```sql
SELECT vout_idx, address, value_sats
FROM tx_outputs
WHERE txid = '\xdeadbeef...'
  AND is_spent = FALSE;
```

---

## Apache AGE Graph Schema

Apache AGE models transaction relationships as a property graph inside PostgreSQL 16.

**Vertex labels:**
- `block` — a Bitcoin block
- `transaction` — a Bitcoin transaction
- `address` — a Bitcoin address

**Edge labels:**
- `sends` — connects addresses to transactions (input relationship)
- `spends` — connects transactions to outputs/addresses

### Sample Cypher Query

Find all addresses connected to a transaction within 2 hops:

```cypher
SELECT * FROM cypher('bitcoin', $$
  MATCH (a:address)-[:sends]->(t:transaction {txid: '...'})
  RETURN a.address, t.txid
$$) AS (address agtype, txid agtype);
```

---

## Related Pages

- [Architecture](architecture.md) — how data flows into these tables
- [API](api.md) — how to query address history and UTXO data
- [Performance](performance.md) — index and partition tuning