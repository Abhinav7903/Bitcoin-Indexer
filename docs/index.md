# Bitcoin Indexer

> High-performance Bitcoin blockchain indexer written in Go. Ingests Bitcoin Core RPC data into PostgreSQL 16 with Apache AGE graph support.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/Abhinav7903/Bitcoin-Indexer/blob/main/LICENSE)
[![Apache AGE](https://img.shields.io/badge/Apache-AGE-orange?style=flat)](https://age.apache.org/)

---

## What Is Bitcoin Indexer?

**Bitcoin Indexer** is an open-source tool that reads every block from a Bitcoin Core node and writes structured, queryable data into PostgreSQL. It is built for developers, data engineers, and infrastructure teams who need fast, reliable access to full Bitcoin blockchain history.

Instead of querying Bitcoin Core directly for every address lookup or transaction search, Bitcoin Indexer maintains a live, indexed copy in PostgreSQL — making queries that would take seconds on raw RPC respond in milliseconds.

---

## Features

- **Full blockchain indexing** — every block, transaction, input, output, and address
- **UTXO tracking** — real-time unspent output set maintained in PostgreSQL
- **Address balance indexing** — denormalized per-address balance table
- **Concurrent pipeline** — configurable worker pool for parallel block fetching
- **Batched PostgreSQL writes** — `pgx` COPY-based inserts for maximum throughput
- **Apache AGE graph support** — graph traversal queries on transaction relationships
- **Partitioned historical tables** — block-height-partitioned tables for scalability
- **Bitcoin Core RPC ingestion** — supports `getblock` verbosity 2
- **Backfill and repair** — dedicated command for historical maintenance
- **Per-stage latency diagnostics** — logs `getblockhash`, `getblock`, `parse`, and `DB write` separately

---

## Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/Abhinav7903/bitcoin-indexer.git
cd bitcoin-indexer

# 2. Copy and edit config
cp config.example.yaml config.yaml

# 3. Start PostgreSQL + Apache AGE
docker compose up -d

# 4. Run migrations
migrate -path migrations -database "postgres://user:password@localhost:5432/btcindex?sslmode=disable" up

# 5. Run the indexer
go run ./cmd/indexer
```

Full setup instructions: [Installation Guide](installation.md)

---

## Architecture

Bitcoin Indexer runs a concurrent fetch-parse-write pipeline:

```
Bitcoin Core RPC
      │
      ▼
Pipeline Workers (concurrent)
   ├── Worker 1: getblockhash → getblock → parse
   └── Worker 2: getblockhash → getblock → parse
      │
      ▼
PostgreSQL 16
   ├── blocks / transactions / inputs / outputs
   ├── address_transactions / address_balances
   └── utxo_set
      │
      ├── HTTP API
      └── Apache AGE Graph Queries
```

Full architecture deep-dive: [Architecture](architecture.md)

---

## Performance

On a machine with NVMe storage and a fully synced Bitcoin Core node:

- Early blocks (< block 200k): **~10 blocks/second**
- Mid-range blocks (300k–500k): **~2–3 blocks/second**
- Recent blocks (800k+): **~0.1–0.3 blocks/second** (large transactions)
- DB write overhead per batch: **15–80ms**

The biggest performance variable is Bitcoin Core's IBD state. During Initial Block Download, `getblockhash` RPC latency can spike to 3–10 seconds per call. This is expected and resolves once the node is fully synced.

Tuning guide: [Performance](performance.md)

---

## Documentation

| Page | Description |
|---|---|
| [Installation](installation.md) | Bitcoin Core, PostgreSQL, AGE setup, Docker, migrations |
| [Architecture](architecture.md) | Pipeline design, workers, batching, database flow |
| [Configuration](configuration.md) | All config options, environment variables, Bitcoin Core tuning |
| [Schema](schema.md) | Database tables, partitioning, indexes, ER overview |
| [Performance](performance.md) | Benchmarks, worker tuning, RPC bottlenecks, disk tips |
| [Troubleshooting](troubleshooting.md) | Common errors, slow stages, RPC issues, PostgreSQL errors |
| [API](api.md) | HTTP API endpoints, address queries, UTXO lookups |

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.22+ |
| Database | PostgreSQL 16 |
| Graph Layer | Apache AGE |
| DB Driver | pgx/v5 |
| Bitcoin Node | Bitcoin Core (RPC verbosity 2) |
| Dev Environment | Docker Compose |

---

## Use Cases

- **Block explorer backends** — index chain data for a web explorer
- **Analytics pipelines** — feed Bitcoin data into BI tools or dashboards
- **Address monitoring** — track balance and transaction history per address
- **UTXO set analysis** — query the unspent output set directly in SQL
- **Graph analysis** — trace transaction flows with Apache AGE graph queries
- **Research and forensics** — full historical data in a standard relational format

---

## Contributing

Contributions are welcome. See the [Contributing Guide](../CONTRIBUTING.md) and `AGENTS.md` for architecture context before making pipeline changes.

---

## License

[MIT License](https://github.com/Abhinav7903/Bitcoin-Indexer/blob/main/LICENSE)
