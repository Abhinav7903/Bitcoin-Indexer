# ₿ Bitcoin Indexer

> A high-performance Bitcoin blockchain indexer written in Go — ingests Bitcoin Core RPC data into PostgreSQL 16 with Apache AGE graph support.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Apache AGE](https://img.shields.io/badge/Apache-AGE-orange?style=flat)](https://age.apache.org/)

---

## Overview

Bitcoin Indexer is an open-source, production-grade tool for indexing the full Bitcoin blockchain into a relational + graph database. It is designed for address/transaction lookups, UTXO tracking, analytics workloads, and historical backfills.

It talks to Bitcoin Core over JSON-RPC, parses blocks concurrently, and writes everything to PostgreSQL using `COPY`-based batch inserts for maximum throughput.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Bitcoin Indexer                          │
│                                                                 │
│   ┌──────────────┐     ┌──────────────────────────────────┐    │
│   │  Bitcoin Core │────▶│         Pipeline Workers         │    │
│   │  (JSON-RPC)  │     │  ┌────────────┐ ┌─────────────┐ │    │
│   └──────────────┘     │  │Block Fetch │ │Block Fetch  │ │    │
│                         │  │ Worker 1   │ │ Worker 2    │ │    │
│   RPC Methods:          │  └─────┬──────┘ └──────┬──────┘ │    │
│   • getblockhash        │        └────────┬───────┘        │    │
│   • getblock (v2)       │                 ▼                │    │
│                         │        ┌────────────────┐        │    │
│                         │        │  Block Parser  │        │    │
│                         │        │  • blocks      │        │    │
│                         │        │  • txs         │        │    │
│                         │        │  • inputs      │        │    │
│                         │        │  • outputs     │        │    │
│                         │        │  • addresses   │        │    │
│                         │        └───────┬────────┘        │    │
│                         └───────────────┼─────────────────┘    │
│                                         ▼                       │
│              ┌──────────────────────────────────────────┐       │
│              │            PostgreSQL 16                 │       │
│              │                                          │       │
│              │  ┌──────────┐  ┌──────────────────────┐ │       │
│              │  │  blocks  │  │     transactions      │ │       │
│              │  └──────────┘  └──────────────────────┘ │       │
│              │  ┌──────────┐  ┌──────────────────────┐ │       │
│              │  │tx_inputs │  │      tx_outputs       │ │       │
│              │  └──────────┘  └──────────────────────┘ │       │
│              │  ┌────────────────┐  ┌────────────────┐ │       │
│              │  │address_txs     │  │   utxo_set     │ │       │
│              │  └────────────────┘  └────────────────┘ │       │
│              │  ┌────────────────┐                      │       │
│              │  │address_balances│                      │       │
│              │  └────────────────┘                      │       │
│              └──────────────────────────────────────────┘       │
│                         │               │                       │
│              ┌──────────▼────┐  ┌───────▼────────────┐         │
│              │   HTTP API    │  │  Apache AGE Graph  │         │
│              │  (REST)       │  │  (Graph Queries)   │         │
│              └───────────────┘  └────────────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

### Indexing Pipeline Flow

```
getblockchaininfo
      │
      ▼
 safe_tip calc  ──▶  (tip - confirmation window)
      │
      ▼
 build batch range [start → end]
      │
      ├──▶ Worker 1: getblockhash → getblock → parse
      ├──▶ Worker 2: getblockhash → getblock → parse
      │              (concurrent, wall time = slowest)
      ▼
 PostgreSQL COPY batch write
      │
      ▼
 next batch range
```

---

## Features

- **Concurrent block ingestion** — configurable worker pool
- **Batched PostgreSQL writes** — uses `pgx` COPY for high throughput
- **Full transaction indexing** — blocks, txs, inputs, outputs, addresses
- **UTXO tracking** — maintained `utxo_set` table
- **Address balance indexing** — denormalized `address_balances`
- **Partitioned tables** — historical tables partitioned by block height
- **Apache AGE graph support** — PostgreSQL 16 graph query layer
- **Backfill / repair command** — separate pipeline for maintenance
- **Detailed RPC timing diagnostics** — per-stage latency logs
- **Safe-tip tracking** — stays behind node tip with confirmation window
- **Graceful shutdown** — clean pipeline teardown on interrupt

---

## Tech Stack

| Component       | Technology                  |
|-----------------|-----------------------------|
| Language        | Go 1.22+                    |
| Database        | PostgreSQL 16               |
| Graph Layer     | Apache AGE                  |
| DB Driver       | `pgx/v5`                    |
| Bitcoin Node    | Bitcoin Core (RPC v2)       |
| Dev Environment | Docker Compose              |

---

## Repository Structure

```text
.
├── cmd/
│   ├── api/              # HTTP API entrypoint
│   ├── backfill/         # Backfill and repair command
│   └── indexer/          # Main indexer entrypoint
│
├── internal/
│   ├── api/              # API handlers and repositories
│   ├── config/           # Config loading (YAML + env)
│   ├── db/               # PostgreSQL batch writer + partitions
│   ├── models/           # Shared domain models
│   └── pipeline/         # Fetch / parse / ingest pipeline
│
├── migrations/           # PostgreSQL schema migrations
│
├── pkg/
│   └── rpc/              # Bitcoin Core JSON-RPC client
│
├── config.example.yaml
├── backfill_config.yaml
├── docker-compose.yml
└── AGENTS.md             # Architecture notes for agents
```

---

## Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose
- Bitcoin Core node (fully synced or syncing)
- [`golang-migrate`](https://github.com/golang-migrate/migrate) CLI

### 1. Clone

```bash
git clone https://github.com/Abhinav7903/bitcoin-indexer.git
cd bitcoin-indexer
```

### 2. Configure

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml`:

```yaml
database_url: "postgres://user:password@localhost:5432/btcindex?sslmode=disable"
rpc_url: "http://rpcuser:rpcpassword@127.0.0.1:8332"
workers: 2
batch_size: 2
start_height: 0
historical_sync: true
```

### 3. Start PostgreSQL + Apache AGE

```bash
docker compose up -d
```

### 4. Run Migrations

```bash
migrate \
  -path migrations \
  -database "postgres://user:password@localhost:5432/btcindex?sslmode=disable" \
  up
```

### 5. Run Indexer

```bash
go run ./cmd/indexer
```

---

## Bitcoin Core Configuration

Add the following to your `bitcoin.conf` for best indexer performance:

```conf
# ── Network ──
mainnet=1
listen=1
server=1
daemon=0

# ── RPC ──
rpcuser=user
rpcpassword=password

# Listen locally + on Tailscale interface
rpcbind=127.0.0.1
rpcbind=<your-tailscale-ip>

# Allow localhost + all Tailscale devices
rpcallowip=127.0.0.1
rpcallowip=<your-tailscale-ip subnet>

rpcport=8332
rpcthreads=8
rpcworkqueue=256

# ── ZMQ ──
zmqpubrawblock=tcp://127.0.0.1:28332
zmqpubrawtx=tcp://127.0.0.1:28333
zmqpubhashblock=tcp://127.0.0.1:28334
zmqpubhashtx=tcp://127.0.0.1:28335

# ── Performance ──
# 16GB cache during initial sync — drop to 2048 after sync done
dbcache=16384
par=7
maxconnections=32
maxuploadtarget=5000

# ── Data ── 
datadir=~/.bitcoin ( change it based on your need)
txindex=1


# ── Mempool ──
maxmempool=512
mempoolexpiry=72

# ── Logging ──
shrinkdebugfile=1

```

> **Note:** During Initial Block Download (`ibd=true`), `getblockhash` RPC latency may spike to several seconds. This is expected — Bitcoin Core is busy validating and writing to LevelDB. The indexer handles this gracefully.

---

## Configuration Reference

All values in `config.yaml` can be overridden by environment variables:

| YAML Key         | Environment Variable | Default   | Description                          |
|------------------|----------------------|-----------|--------------------------------------|
| `database_url`   | `DATABASE_URL`       | —         | PostgreSQL connection string         |
| `rpc_url`        | `RPC_URL`            | —         | Bitcoin Core RPC URL                 |
| `workers`        | `WORKERS`            | `2`       | Concurrent block fetch workers       |
| `batch_size`     | `BATCH_SIZE`         | `2`       | Blocks per batch                     |
| `start_height`   | `START_HEIGHT`       | `0`       | Block height to start from           |
| `historical_sync`| `HISTORICAL_SYNC`    | `true`    | Enable historical sync mode          |

> Keep `workers` equal to `batch_size` during IBD. Extra workers sit idle and add unnecessary RPC pressure to an already-busy node.

---

## Database Schema

Tables maintained by the indexer:

| Table                  | Description                                  |
|------------------------|----------------------------------------------|
| `blocks`               | Block header data                            |
| `transactions`         | All transactions per block                   |
| `tx_inputs`            | Transaction inputs (prev outpoint reference) |
| `tx_outputs`           | Transaction outputs (value, scriptpubkey)    |
| `address_transactions` | Denormalized address → tx mapping            |
| `utxo_set`             | Unspent transaction output set               |
| `address_balances`     | Denormalized per-address balance             |

Large historical tables are **partitioned by block height** for query performance and maintainability. Partitions are created automatically by the batch writer as needed.

---

## Running the API

```bash
go run ./cmd/api
```

The API reads from indexed PostgreSQL tables. Address history queries use the denormalized `address_transactions` table to avoid expensive joins.

---

## Running Backfill

Use the backfill command for repair jobs, historical reprocessing, or maintenance:

```bash
go run ./cmd/backfill
```

Configure separately via `backfill_config.yaml`. Do not mix backfill into the live indexer loop.

---

## Performance & Diagnostics

The indexer emits per-stage timing logs for every block:

```text
Block 140231: RPC total=10.084s getblockhash=10.062s getblock=21ms parse=1ms (97 txs)
Batch 140231-140232: fetched 2 blocks (228 txs) in 10.089s wall time, DB write in 125ms
```

Use these to pinpoint exactly where time is being spent:

| Pattern | Bottleneck | Action |
|---|---|---|
| `getblockhash=10s getblock=20ms` | Bitcoin Core IBD / RPC queue | Wait for IBD, tune `rpcthreads`, check `dbcache` |
| `getblockhash=5ms getblock=8s` | Verbose block fetch / slow disk | Check node disk I/O, CPU, block size |
| `parse=2s` | Parser / JSON decode CPU | Check Go CPU usage, JSON overhead |
| `DB write in 5s` | PostgreSQL write pressure | Check WAL, indexes, partitions, disk |

> **Batch wall time** is the elapsed time until the *slowest* worker in the batch finishes — not the sum of all workers.

---

## Operational Notes

For fast historical sync:

- Run Bitcoin Core and PostgreSQL on **SSD or NVMe** storage
- Set a high `dbcache` in `bitcoin.conf` (16GB+ if RAM allows)
- Keep `workers = batch_size` during IBD
- For PostgreSQL initial load only, you can temporarily set:

```conf
wal_level = minimal
fsync = off
```

> ⚠️ Only use `fsync=off` for initial historical sync when you understand the durability tradeoff. Re-enable safe settings immediately after sync completes.

---

## Log Reference

```text
# Node sync state
RPC blockchain info | blocks=885293 headers=948406 ibd=true
Bitcoin node syncing | blocks=885293 headers=948406 remaining=63113

# Batch progress
Ingesting blocks 140231 -> 140232 | safe_tip=885283 blocks=885293 headers=948406
Batch 140231-140232: fetched 2 blocks (228 txs) in 10.089s wall time, DB write in 125ms

# Per-block RPC timing
Block 140231: RPC total=10.084s getblockhash=10.062s getblock=21ms parse=1ms (97 txs)
```

---

## Roadmap

- [ ] Raw block RPC parsing (skip verbose JSON)
- [X] Direct `blk*.dat` file ingestion (for faster historical sync but can be complex)
- [X] Reorg-safe rollback pipeline (hard to implement and test properly)
- [ ] Prometheus metrics endpoint
- [ ] Swagger / OpenAPI docs
- [ ] WebSocket new block notifications
- [ ] Multi-node RPC failover
- [ ] Horizontal ingestion workers
- [ ] Direct LevelDB access research

---

## Contributing

Contributions are welcome! Suggested workflow:

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make and test your changes locally
4. Open a pull request with a clear description

Please check `AGENTS.md` for architecture and operational context before making pipeline changes.

---

## License

[MIT License](LICENSE)

---

## Acknowledgements

- [Bitcoin Core](https://github.com/bitcoin/bitcoin)
- [btcsuite](https://github.com/btcsuite)
- [PostgreSQL](https://www.postgresql.org/)
- [Apache AGE](https://age.apache.org/)
- [pgx](https://github.com/jackc/pgx)