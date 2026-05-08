# Configuration

> Complete reference for all Bitcoin Indexer configuration options, environment variables, and Bitcoin Core tuning.

---

## Config File

Bitcoin Indexer reads `config.yaml` on startup. Environment variables override any values in the file.

Default config location: `./config.yaml` (relative to the binary).

### Full Example

```yaml
database_url: "postgres://user:password@localhost:5432/btcindex?sslmode=disable"
rpc_url: "http://rpcuser:rpcpassword@127.0.0.1:8332"
workers: 2
batch_size: 2
start_height: 0
historical_sync: true
```

---

## Indexer Options

### `database_url`

**Type:** string  
**Environment variable:** `DATABASE_URL`  
**Required:** yes

PostgreSQL connection string. Supports standard `libpq` parameters.

```yaml
database_url: "postgres://user:password@localhost:5432/btcindex?sslmode=disable"
```

For production, use `sslmode=require` and a certificate.

---

### `rpc_url`

**Type:** string  
**Environment variable:** `RPC_URL`  
**Required:** yes

Bitcoin Core JSON-RPC endpoint including credentials.

```yaml
rpc_url: "http://rpcuser:rpcpassword@127.0.0.1:8332"
```

If Bitcoin Core is on a remote host, replace `127.0.0.1` with the host IP and make sure `rpcallowip` in `bitcoin.conf` permits that address.

---

### `workers`

**Type:** integer  
**Environment variable:** `WORKERS`  
**Default:** `2`  
**Recommended range:** `2–8`

Number of concurrent block fetch workers. Each worker independently calls `getblockhash` and `getblock` for one block in the current batch.

```yaml
workers: 2
```

**Rule:** Keep `workers` equal to `batch_size` during IBD.

Setting `workers` higher than `batch_size` wastes goroutines — the extra workers have no blocks to fetch. Setting `workers` much higher than `batch_size` during IBD also adds unnecessary RPC pressure to a Bitcoin Core node that is already busy validating.

```
# Good during IBD
workers: 2
batch_size: 2

# Wasteful during IBD (10 workers, only 2 blocks per batch)
workers: 10
batch_size: 2
```

After IBD, increasing both `workers` and `batch_size` together (e.g. `8/8` or `16/16`) can improve throughput.

---

### `batch_size`

**Type:** integer  
**Environment variable:** `BATCH_SIZE`  
**Default:** `2`  
**Recommended range:** `2–16`

Number of blocks fetched and written per batch cycle.

```yaml
batch_size: 2
```

A larger batch size amortizes PostgreSQL write overhead across more blocks, which improves throughput for high-transaction blocks. However, larger batches also increase memory usage and mean you must wait for more blocks before any are committed to the database.

Suggested values by phase:

| Phase | Recommended `batch_size` |
|---|---|
| IBD (early blocks, < 300k) | 2–4 |
| IBD (mid blocks, 300k–600k) | 4–8 |
| IBD (recent blocks, 600k+) | 2–4 (large blocks need more RAM) |
| Post-IBD live sync | 1–2 |

---

### `start_height`

**Type:** integer  
**Environment variable:** `START_HEIGHT`  
**Default:** `0` (or last indexed height from DB)

Block height to start indexing from. If the database already contains indexed blocks, the indexer will automatically resume from the last known height unless this is explicitly overridden.

```yaml
start_height: 500000
```

Use this to start from a checkpoint or to re-index a specific range.

---

### `historical_sync`

**Type:** boolean  
**Environment variable:** `HISTORICAL_SYNC`  
**Default:** `true`

Enables historical sync mode. When `true`, the indexer continuously fetches blocks from `start_height` forward until it reaches the safe tip.

```yaml
historical_sync: true
```

---

## Environment Variable Overrides

All config options can be set via environment variables. Environment variables take precedence over `config.yaml`.

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/btcindex?sslmode=disable"
export RPC_URL="http://rpcuser:rpcpassword@127.0.0.1:8332"
export WORKERS=4
export BATCH_SIZE=4
export START_HEIGHT=0
export HISTORICAL_SYNC=true
```

---

## Bitcoin Core Configuration

The indexer's performance is tightly coupled to Bitcoin Core's configuration. These settings go in `~/.bitcoin/bitcoin.conf`.

### Recommended `bitcoin.conf`

```conf
# Enable RPC server
server=1

# Full transaction index (required)
txindex=1

# Memory cache for block validation (adjust to your RAM)
# Rule of thumb: total_RAM_in_MB - 4096 - postgres_RAM
dbcache=16384

# RPC concurrency
rpcthreads=8
rpcworkqueue=64

# Network
maxconnections=64
maxuploadtarget=5000

# Block/signature validation threads
par=8

# Mempool
maxmempool=512
mempoolexpiry=72

# Log management
shrinkdebugfile=1
```

### Key Parameters Explained

#### `dbcache`

The most important setting for IBD performance. This is the in-memory cache Bitcoin Core uses for block validation and UTXO set lookups. A larger cache means fewer disk reads during block validation, which directly reduces `getblockhash` RPC latency.

| RAM | Recommended `dbcache` |
|---|---|
| 8 GB | 4096 |
| 16 GB | 8192 |
| 32 GB | 16384 |
| 64 GB+ | 32768 |

After IBD is complete, `dbcache` can be reduced as the UTXO set is fully built and lookups become faster.

#### `rpcthreads`

Number of threads Bitcoin Core dedicates to handling RPC requests. During IBD, multiple indexer workers compete for RPC access. Setting this to 8 allows up to 8 concurrent RPC calls to be processed.

#### `rpcworkqueue`

Maximum number of RPC requests queued before Bitcoin Core starts rejecting new ones. A larger queue prevents `HTTP 429: Work queue depth exceeded` errors under high RPC load.

#### `par`

Number of script verification threads. More threads speed up block validation during IBD, which indirectly reduces `getblockhash` stalls.

---

## PostgreSQL Tuning

For initial historical sync, PostgreSQL can be temporarily tuned for bulk write throughput:

```conf
# postgresql.conf (or set via ALTER SYSTEM)
shared_buffers = 4GB
work_mem = 256MB
maintenance_work_mem = 2GB
checkpoint_completion_target = 0.9
wal_buffers = 64MB
max_wal_size = 4GB
```

For initial load only (understand the durability tradeoff before using):

```conf
wal_level = minimal
fsync = off
synchronous_commit = off
```

> ⚠️ **Never use `fsync=off` in production.** Re-enable it after the initial sync is complete. Power loss with `fsync=off` can corrupt the database.

---

## Backfill Config

The backfill command uses a separate config file: `backfill_config.yaml`.

```yaml
database_url: "postgres://user:password@localhost:5432/btcindex?sslmode=disable"
rpc_url: "http://rpcuser:rpcpassword@127.0.0.1:8332"
workers: 4
batch_size: 4
start_height: 140000
historical_sync: true
```

Keep backfill jobs separate from the live indexer to avoid competing for RPC and database connections.

---

## Related Pages

- [Installation](installation.md) — full setup guide
- [Performance](performance.md) — tuning for speed
- [Troubleshooting](troubleshooting.md) — common configuration errors
