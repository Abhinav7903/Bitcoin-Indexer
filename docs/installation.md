# Installation Guide

> How to set up Bitcoin Indexer from scratch — Bitcoin Core, PostgreSQL, Apache AGE, migrations, and the indexer itself.

---

## Prerequisites

Before installing Bitcoin Indexer, you need:

| Requirement | Version | Notes |
|---|---|---|
| Go | 1.22+ | [Download](https://go.dev/dl/) |
| Docker | Latest | For PostgreSQL + AGE |
| Docker Compose | v2+ | Included in Docker Desktop |
| Bitcoin Core | 25+ | Fully synced or syncing node |
| golang-migrate | Latest | For running DB migrations |

---

## Step 1: Install Bitcoin Core

Bitcoin Indexer requires a running Bitcoin Core node with `txindex=1` enabled.

### Download Bitcoin Core

```bash
# Linux (example - check https://bitcoincore.org for latest version)
wget wget https://bitcoincore.org/bin/bitcoin-core-${BTC_VERSION}
tar -xzf bitcoin-${BTC_VERSION}-linux-gnu.tar.gz
sudo install -m 0755 -o root -g root -t /usr/local/bin bitcoin-${BTC_VERSION}/bin/*
```

### Configure Bitcoin Core

Create or edit `~/.bitcoin/bitcoin.conf`:

```conf
# Required for transaction indexing
server=1
txindex=1

# Performance tuning (adjust to your available RAM)
dbcache=16384
rpcthreads=8
rpcworkqueue=64

# Network
maxconnections=64
par=8

# Mempool
maxmempool=512
mempoolexpiry=72

# RPC credentials
rpcuser=youruser
rpcpassword=yourpassword
rpcbind=127.0.0.1
rpcallowip=127.0.0.1
```

> **Important:** `dbcache` is the single biggest performance lever for Bitcoin Core during Initial Block Download. Set it as high as your RAM allows (leave 4GB+ for the OS and PostgreSQL).

### Start Bitcoin Core

```bash
bitcoind -daemon
```

Check sync status:

```bash
bitcoin-cli getblockchaininfo
```

You will see `"initialblockdownload": true` while syncing. The indexer works during IBD but RPC latency will be higher.

---

## Step 2: Install Go

```bash
# Download Go 1.22+
wget https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify
go version
```

---

## Step 3: Install golang-migrate

```bash
# Install the migrate CLI tool
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Verify
migrate --version
```

---

## Step 4: Clone the Repository

```bash
git clone https://github.com/Abhinav7903/bitcoin-indexer.git
cd bitcoin-indexer
```

---

## Step 5: Start PostgreSQL with Apache AGE

The project ships with a `docker-compose.yml` that runs PostgreSQL 16 with Apache AGE pre-installed.

```bash
docker compose up -d
```

Verify PostgreSQL is running:

```bash
docker compose ps
docker compose logs postgres
```

> **PostgreSQL 16 is required.** Apache AGE is not compatible with PostgreSQL 17+. Do not upgrade the PostgreSQL version in `docker-compose.yml`.

### Manual PostgreSQL Setup (without Docker)

If you are running PostgreSQL 16 directly on the host:

1. Install PostgreSQL 16
2. Install Apache AGE for PostgreSQL 16 from [age.apache.org](https://age.apache.org/)
3. Create the database:

```sql
CREATE DATABASE btcindex;
\c btcindex
CREATE EXTENSION age;
LOAD 'age';
SET search_path = ag_catalog, "$user", public;
```

---

## Step 6: Configure the Indexer

Copy the example config:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml`:

```yaml
database_url: "postgres://user:password@localhost:5432/btcindex?sslmode=disable"
rpc_url: "http://youruser:yourpassword@127.0.0.1:8332"
workers: 2
batch_size: 2
start_height: 0
historical_sync: true
```

See [Configuration](configuration.md) for all available options.

---

## Step 7: Run Database Migrations

```bash
migrate \
  -path migrations \
  -database "postgres://user:password@localhost:5432/btcindex?sslmode=disable" \
  up
```

This creates all tables, partitions, and indexes. Migrations are safe to re-run.

Verify the schema was created:

```bash
psql "postgres://user:password@localhost:5432/btcindex" -c "\dt"
```

You should see tables: `blocks`, `transactions`, `tx_inputs`, `tx_outputs`, `address_transactions`, `utxo_set`, `address_balances`.

---

## Step 8: Run the Indexer

```bash
# Run directly with Go
go run ./cmd/indexer

# Or build and run
go build -o indexer ./cmd/indexer
./indexer
```

You should see logs like:

```text
RPC blockchain info | blocks=886457 headers=948409 ibd=true
Bitcoin node syncing | blocks=886457 headers=948409 remaining=61952
Ingesting blocks 0 -> 1 | safe_tip=886447 blocks=886457 headers=948409
Batch 0-1: fetched 2 blocks (2 txs) in 48ms wall time, DB write in 15ms
```

---

## Step 9: Run the API (Optional)

```bash
go run ./cmd/api
```

The API server starts and reads from the indexed PostgreSQL tables. See [API](api.md) for endpoint documentation.

---

## Starting from a Specific Height

To skip already-indexed blocks or start from a checkpoint:

```bash
export START_HEIGHT=500000
go run ./cmd/indexer
```

Or set `start_height` in `config.yaml`.

---

## Verifying the Index

Check that blocks are being written:

```bash
psql "postgres://user:password@localhost:5432/btcindex" \
  -c "SELECT height, hash, tx_count FROM blocks ORDER BY height DESC LIMIT 10;"
```

Check transaction count:

```bash
psql "postgres://user:password@localhost:5432/btcindex" \
  -c "SELECT COUNT(*) FROM transactions;"
```

---

## Stopping the Indexer

The indexer handles `SIGINT` and `SIGTERM` gracefully. Press `Ctrl+C` and it will finish the current batch before shutting down.

---

## Troubleshooting Installation

**`getblockhash` is very slow** — Bitcoin Core is still in IBD. This is normal. See [Troubleshooting](troubleshooting.md).

**`migrate: error: no change`** — Migrations already applied. This is fine.

**`connection refused` on PostgreSQL** — Check `docker compose ps`. If the container is not running, check `docker compose logs postgres`.

**`connection refused` on RPC** — Verify `rpcuser`, `rpcpassword`, and `rpcbind` in `bitcoin.conf`. Restart `bitcoind` after config changes.

---

## Next Steps

- [Configuration](configuration.md) — tune workers, batching, and Bitcoin Core
- [Architecture](architecture.md) — understand the pipeline design
- [Performance](performance.md) — optimize for your hardware
