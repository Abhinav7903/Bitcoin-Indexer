# Bitcoin Indexer

A Go-based Bitcoin indexer that reads blocks from Bitcoin Core RPC and stores indexed chain data in PostgreSQL 16 with Apache AGE available for graph tracing.

The indexer is designed for historical sync and address/transaction lookup workloads. It fetches Bitcoin blocks from RPC, parses transactions, writes blocks/transactions/inputs/outputs in batches, and keeps denormalized tables for faster address queries.

## Tech Stack

- Go 1.22+
- PostgreSQL 16 (Required for Apache AGE; versions > 16 are not supported)
- Apache AGE for graph queries (Compatible with PostgreSQL 16)
- `pgx/v5` for PostgreSQL access
- Bitcoin Core RPC with `getblock` verbosity `2`
- Docker Compose for local PostgreSQL/AGE

## Repository Structure

```text
.
├── cmd/
│   ├── api/          # HTTP API entrypoint
│   ├── backfill/     # Backfill/repair command
│   └── indexer/      # Main block indexer entrypoint
├── internal/
│   ├── api/          # API handlers, models, and repository queries
│   ├── config/       # YAML and environment config loading
│   ├── db/           # Batch database writer and partition handling
│   ├── models/       # Shared database/domain models
│   └── pipeline/     # Block fetch, parse, and ingest pipeline
├── migrations/       # PostgreSQL schema migrations
├── pkg/
│   └── rpc/          # Bitcoin Core JSON-RPC client
├── config.yaml       # Local indexer config
├── backfill_config.yaml
├── docker-compose.yml
└── AGENTS.md         # Architecture and operational notes for agents
```

## The RPC Latency Problem

During indexing, some batches were taking several seconds even though parsing and database writes were fast.

Example log:

```text
Block 140231: RPC total=10.084s getblockhash=10.062s getblock=21ms parse=1ms (97 txs)
Batch 140231-140232: fetched 2 blocks (228 txs) in 10.089s wall time, DB write in 125ms
RPC blockchain info | blocks=885293 headers=948406 ibd=true
```

This showed the real bottleneck:

- `DB write` was fast.
- `parse` was very fast.
- `getblock` was fast.
- `getblockhash` was slow.
- Bitcoin Core was still in Initial Block Download: `ibd=true`.

So the delay was not caused by PostgreSQL or transaction parsing. The delay came from Bitcoin Core RPC waiting while the node was still syncing and busy validating blocks.

## How It Was Diagnosed

The block log was changed to split RPC timing into separate stages:

```text
RPC total=...
getblockhash=...
getblock=...
parse=...
```

This is useful because `RPC total` alone does not show which RPC method is slow. After splitting the log, it became clear that `getblockhash` was sometimes waiting 3-10 seconds while `getblock` returned in milliseconds.

The batch log also says `wall time`:

```text
Batch 140231-140232: fetched 2 blocks (228 txs) in 10.089s wall time, DB write in 125ms
```

This matters because blocks are fetched concurrently. Batch fetch time is not the sum of all block RPC calls; it is the elapsed wall-clock time until the slowest block in that batch finishes.

## How To Reduce Latency

### 1. Keep workers aligned with batch size

If `batch_size` is `2`, use `workers: 2`.

Example:

```yaml
workers: 2
batch_size: 2
```

Using `workers: 12` with `batch_size: 2` does not help because only two blocks exist in each batch. Ten workers stay idle. Increasing workers while Bitcoin Core is in IBD can also add more RPC pressure to a node that is already busy.

### 2. Let Bitcoin Core finish IBD when possible

Your logs showed:

```text
ibd=true
```

When Bitcoin Core is still syncing, RPC can pause while validation and disk work are happening. The indexer will behave much better after the node is fully synced.

### 3. Tune Bitcoin Core RPC capacity

Add or adjust these values in `bitcoin.conf`:

```conf
rpcthreads=8
rpcworkqueue=64
dbcache=8192
```

Then restart `bitcoind`.

What these help with:

- `rpcthreads`: lets Bitcoin Core process more RPC requests concurrently.
- `rpcworkqueue`: allows more pending RPC requests before queue pressure appears.
- `dbcache`: gives Bitcoin Core more cache for validation and block/index data.

Do not set these blindly too high on a small machine. If the server has limited RAM, reduce `dbcache`.

### 4. Watch the split timing logs

Use the slow block logs to decide where the bottleneck is:

```text
getblockhash=10s getblock=20ms parse=1ms
```

Bitcoin Core RPC/node sync bottleneck.

```text
getblockhash=5ms getblock=8s parse=1ms
```

Verbose block fetch is slow. This can happen with large blocks, slow disk, or overloaded Bitcoin Core.

```text
getblockhash=5ms getblock=30ms parse=2s
```

Parser/indexer CPU issue.

```text
DB write in 5s
```

PostgreSQL write/index/partition issue.

## Configuration

The indexer reads `config.yaml` and then allows environment variables to override it.

Example `config.yaml`:

```yaml
database_url: "postgres://user:password@localhost:5432/btcindex?sslmode=disable"
rpc_url: "http://rpcuser:rpcpassword@btcnode:8332"
workers: 2
batch_size: 2
start_height: 0
historical_sync: true
```

Supported environment overrides:

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/btcindex?sslmode=disable"
export RPC_URL="http://rpcuser:rpcpassword@localhost:8332"
export WORKERS=2
export BATCH_SIZE=2
export START_HEIGHT=0
export HISTORICAL_SYNC=true
```

## Database Setup

Start PostgreSQL with Apache AGE:

```bash
docker-compose up -d
```

Run migrations:

```bash
migrate -path migrations -database "postgres://user:password@localhost:5432/btcindex?sslmode=disable" up
```

The schema uses partitioned historical tables and hot lookup tables:

- `blocks`
- `transactions`
- `tx_outputs`
- `tx_inputs`
- `address_transactions`
- `utxo_set`
- `address_balances`

Large historical tables are partitioned by block height. The writer creates missing partitions as needed.

## Running The Indexer

Build:

```bash
go build -o indexer ./cmd/indexer
```

Run:

```bash
./indexer
```

Or run directly:

```bash
go run ./cmd/indexer
```

The indexer starts from the latest indexed height in PostgreSQL. If the database is empty, it starts from block `0`. You can override the start height with:

```bash
export START_HEIGHT=140000
```

## How Indexing Works

1. The indexer checks Bitcoin Core sync state with `getblockchaininfo`.
2. It stays behind the node tip by a safety confirmation window.
3. It builds a batch range from `start_height` to `end_height`.
4. Workers fetch blocks from Bitcoin Core RPC.
5. Each block is parsed into block, transaction, input, output, and address rows.
6. The database writer saves the whole batch.
7. The next batch starts after the previous one completes.

The important performance rule is: avoid row-by-row processing. Fetch and write in batches.

## Important Logs

Blockchain state:

```text
RPC blockchain info | blocks=885293 headers=948406 ibd=true
Bitcoin node syncing | blocks=885293 headers=948406 remaining=63113
```

Batch progress:

```text
Ingesting blocks 140231 -> 140232 | safe_tip=885283 blocks=885293 headers=948406
Batch 140231-140232: fetched 2 blocks (228 txs) in 10.089s wall time, DB write in 125ms
```

Slow block details:

```text
Block 140231: RPC total=10.084s getblockhash=10.062s getblock=21ms parse=1ms (97 txs)
```

## Running The API

Build/run the API command:

```bash
go run ./cmd/api
```

The API uses the indexed PostgreSQL tables. Address history should query the denormalized `address_transactions` table instead of joining raw transaction tables for every request.

## Backfill

The backfill command uses `backfill_config.yaml`.

Run:

```bash
go run ./cmd/backfill
```

Use this command for repair or historical maintenance tasks that should not be mixed into the live indexer loop.

## Operational Notes

For historical sync, Bitcoin Core and PostgreSQL are both heavy users of disk I/O. Best results come from:

- running Bitcoin Core and PostgreSQL on fast SSD/NVMe storage;
- giving Bitcoin Core enough `dbcache`;
- avoiding too many RPC workers during IBD;
- keeping batch writes enabled;
- monitoring `getblockhash`, `getblock`, `parse`, and `DB write` timings separately.

For very large historical loads, PostgreSQL can be tuned temporarily:

```conf
wal_level = minimal
fsync = off
```

Only use those settings for initial sync when you understand the durability tradeoff. Re-enable safe settings after sync.

## Quick Troubleshooting

If indexing is slow, first check the slow block log.

Slow `getblockhash`:

```text
getblockhash=10s getblock=20ms
```

Bitcoin Core is likely busy, still syncing, or RPC is queued.

Slow `getblock`:

```text
getblockhash=5ms getblock=8s
```

Verbose block fetch is slow. Check disk, CPU, Bitcoin Core load, and RPC concurrency.

Slow DB write:

```text
DB write in 5s
```

Check PostgreSQL indexes, partitions, disk I/O, and batch size.

Slow parse:

```text
parse=2s
```

Check parser CPU and JSON decoding overhead.
