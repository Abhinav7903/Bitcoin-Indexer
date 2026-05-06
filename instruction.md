# Instructions for AI Agents Working on This Bitcoin Indexer

## Goal
Build and maintain a high-performance Bitcoin indexer in Go using PostgreSQL 16 and Apache AGE. Follow the architecture described in `BTc indexer.pdf` and keep the implementation aligned with the SQL schema in `migrations/000001_initial_schema.up.sql`.

## Module and Repository Rules
- Go module path is `github.com/Abhinav7903/bitcoin-indexer`.
- Keep imports under that module path so the project can be exported to GitHub.
- Do not create extra migration versions unless the user asks. For initial schema work, update `000001_initial_schema.up.sql` and `000001_initial_schema.down.sql`.
- Use Go 1.22+ patterns and `github.com/jackc/pgx/v5` for PostgreSQL access.

## Database Architecture
Use these tables as the source of truth:
- `blocks`: one row per Bitcoin block.
- `transactions`: partitioned by `block_height`.
- `tx_outputs`: partitioned historical output table with spend status.
- `tx_inputs`: partitioned historical input table.
- `address_transactions`: denormalized fast-path address history table.
- `utxo_set`: current live UTXO table; never compute production balances from `tx_outputs`.
- `address_balances`: cached balance table updated in batches.
- `index_state`: checkpoint table for resume/recovery.

## Ingestion Strategy
Always use two-phase, batch ingestion:
1. Fetch blocks in parallel from Bitcoin Core RPC with `getblock` verbosity `2`.
2. Parse blocks into in-memory batches of blocks, transactions, inputs, outputs, and address history rows.
3. Bulk `COPY` rows with pgx. Do not insert row-by-row.
4. Bulk mark spent outputs through a temporary table and a single `UPDATE ... FROM` join.
5. Bulk delete spent rows from `utxo_set` and bulk insert new address-bearing outputs.
6. Update `address_balances` in batch.
7. Update `index_state` after the DB transaction succeeds.

## Indexing and Partitioning Rules
- Partition large historical tables by `block_height` in 100,000-block ranges.
- Use BRIN indexes for chronological columns: `block_height`, `block_time`.
- Use BTREE indexes for exact lookups: `txid`, `address`, `(prev_txid, prev_vout)`.
- Use Apache AGE only for 2+ hop wallet tracing. SQL is preferred for 1-hop tracing and normal queries.

## Code Expectations
- Keep the parser tolerant of Bitcoin Core JSON differences, but fail clearly on missing critical fields like block hash or transaction id.
- Store historical outputs even when they have no address. Only insert address-bearing outputs into `utxo_set`.
- Create `address_transactions` receiver rows for output addresses and sender rows when input `prevout` data includes an address and value.
- Preserve batch order by block height before writing so checkpoints advance correctly.
- Run `gofmt ./...` and `go build ./...` before handing work back.

## Developer Commands
```bash
docker-compose up -d
migrate -path migrations -database "$DATABASE_URL" up
go build -o indexer ./cmd/indexer
./indexer
```

Required environment:
```bash
export DATABASE_URL="postgres://hornet:H0rnSt@r@localhost:5432/btc_indexer?sslmode=disable"
export RPC_URL="http://user:password@localhost:8332"
```

Optional tuning:
```bash
export WORKERS=10
export BATCH_SIZE=100
export START_HEIGHT=0
```
