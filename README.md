# Bitcoin Indexer

A high-performance Bitcoin indexer built with Go, PostgreSQL 16, and Apache AGE.

## Prerequisites

- Go 1.22+
- PostgreSQL 16 with Apache AGE extension
- Bitcoin Core Node with ZMQ and RPC enabled

## Development Setup

1. Start the database:
   ```bash
   docker-compose up -d
   ```

2. Run migrations:
   ```bash
   migrate -path migrations -database "postgres://hornet:H0rnSt@r@localhost:5432/btc_indexer?sslmode=disable" up
   ```

3. Build and Run:
   ```bash
   export DATABASE_URL="postgres://hornet:H0rnSt@r@localhost:5432/btc_indexer?sslmode=disable"
   export RPC_URL="http://user:password@localhost:8332"
   go build -o indexer ./cmd/indexer
   ./indexer
   ```

3. Build and Run:
   ```bash
   export DATABASE_URL="postgres://user:password@localhost:5432/bitcoin_indexer"
   export RPC_URL="http://user:password@localhost:8332"
   go build -o indexer ./cmd/indexer
   ./indexer
   ```

## Architecture

See [AGENTS.md](./AGENTS.md) for detailed architecture and database strategy.
