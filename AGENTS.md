# Agent Guide: BitCoin Indexer

## Architecture & Tech Stack
- **Language**: Go 1.22+ (using `pgx/v5`)
- **Database**: PostgreSQL 16 + Apache AGE (graph layer for multi-hop tracing)
- **Source**: Bitcoin Core Node (ZMQ + RPC `getblock verbose=2`)
- **Pipeline**: Parallel Block Fetcher -> Batch UTXO Resolver -> Batch Writer (`pgx` COPY)

## Database Strategy (High Signal)
- **Two-Phase Ingestion**: Never process row-by-row.
    1. Phase 1: Bulk `COPY` all outputs.
    2. Phase 2: Single bulk `UPDATE` for spent status via a temporary table join.
- **Partitioning**: Large tables are partitioned by `block_height` in ranges of **100,000 blocks**.
- **Indexing**:
    - **BRIN**: Mandatory for `block_height` and `block_time`. It is 1000x smaller than BTREE for these columns.
    - **BTREE**: Use only for `txid`, `address`, or exact value lookups.
- **Hot Path Tables**:
    - `address_transactions`: Denormalized history table. Query this for "Last N tx" to achieve <20ms latency with zero joins.
    - `utxo_set`: Materialized live UTXOs. Never compute balance from `tx_outputs` in production.
    - `address_balances`: Cached balances updated in batch.

## Developer Commands
- **Build**: `go build -o indexer ./cmd/indexer`
- **Environment**: Requires `DATABASE_URL` and `RPC_URL`.
- **Infrastructure**: Use `docker-compose.yml` with `apache/age:PG16_latest` for local dev.

## Operational Constraints
- **Initial Sync Tuning**: For historical load, set `wal_level = minimal` and `fsync = off`. Re-enable after sync.
- **Graph Queries**: Use Apache AGE **only** for 2+ hop tracing. SQL is faster for 1-hop or basic queries.
- **Storage**: Full chain (~870k blocks) requires ~1.4TB; 3-year pruned index requires ~600GB.
