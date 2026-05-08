# Troubleshooting

> Diagnosis and fixes for common Bitcoin Indexer problems — slow RPC, PostgreSQL errors, connection failures, and more.

---

## How to Read the Logs First

Before diagnosing, check which stage is slow:

```text
Block 140231: RPC total=10.084s getblockhash=10.062s getblock=21ms parse=1ms (97 txs)
```

| Log field | What it measures |
|---|---|
| `getblockhash` | Time waiting for Bitcoin Core to return block hash |
| `getblock` | Time to fetch full decoded block JSON |
| `parse` | Time to parse JSON into DB rows |
| `DB write` | Time for PostgreSQL COPY write |

The slow field points you directly to the bottleneck.

---

## Slow `getblockhash` (most common)

### Symptom

```text
Block 140231: RPC total=10.084s getblockhash=10.062s getblock=21ms parse=1ms
```

`getblockhash` is 90%+ of total time.

### Cause

Bitcoin Core RPC is stalling. This almost always means Bitcoin Core is in IBD (`ibd=true`) and is busy with:

- LevelDB compaction
- Block validation
- Disk writes for the UTXO set
- RPC queue pressure (too many pending requests)

### Fixes

**1. Check IBD status:**

```bash
bitcoin-cli getblockchaininfo | grep initialblockdownload
```

If `true`, this latency is expected and will improve as the node syncs.

**2. Increase `dbcache`:**

```conf
# bitcoin.conf
dbcache=16384
```

Restart `bitcoind`. A larger cache means fewer LevelDB disk reads during validation, which reduces RPC stall time.

**3. Increase RPC capacity:**

```conf
rpcthreads=8
rpcworkqueue=64
```

**4. Reduce `workers` during IBD:**

```yaml
workers: 2
batch_size: 2
```

More workers send more concurrent RPC requests to an already-busy node, making the stalls worse.

---

## Slow `getblock`

### Symptom

```text
getblockhash=5ms getblock=8s parse=1ms
```

`getblock` is slow but `getblockhash` is fast.

### Cause

Fetching the full verbose block (`verbosity=2`) is slow. Common causes:

- Very large block (recent blocks can be 3MB+ with thousands of transactions)
- Slow disk on Bitcoin Core host
- Bitcoin Core CPU saturated
- Network latency if Bitcoin Core is on a remote host

### Fixes

**1. Check disk I/O on the Bitcoin Core host:**

```bash
iostat -x 1 5
```

If `%util` on the Bitcoin Core disk is near 100%, the disk is the bottleneck.

**2. Ensure Bitcoin Core is on SSD/NVMe:**

Spinning HDD will cause `getblock` to take 5–30 seconds for large recent blocks.

**3. Reduce concurrent workers:**

```yaml
workers: 2
batch_size: 2
```

**4. Check Bitcoin Core CPU:**

```bash
top -p $(pgrep bitcoind)
```

---

## Slow DB Write

### Symptom

```text
Batch 141045-141046: fetched 2 blocks (87 txs) in 45ms wall time, DB write in 5s
```

DB write is slow relative to fetch time.

### Causes and Fixes

**Missing partition:**

The batch writer creates partitions automatically, but if there is a bug, a write can hit an unpartitioned fallback. Check:

```sql
SELECT tablename FROM pg_tables WHERE tablename LIKE 'transactions_%';
```

**PostgreSQL disk I/O:**

```bash
iostat -x 1 5
# Check the disk where PostgreSQL data directory lives
```

**Index maintenance overhead:**

During heavy bulk writes, indexes slow things down. For initial sync only:

```sql
-- Drop non-critical indexes before sync
-- Re-create after sync completes
```

**WAL bottleneck:**

```conf
# postgresql.conf
synchronous_commit = off
wal_buffers = 64MB
max_wal_size = 4GB
checkpoint_completion_target = 0.9
```

---

## Slow Parse

### Symptom

```text
getblockhash=5ms getblock=30ms parse=2s
```

### Cause

Go CPU-bound: JSON decode or struct allocation is slow.

### Fixes

**Check Go CPU:**

```bash
top -p $(pgrep indexer)
```

**Check GOMAXPROCS:**

```bash
# Should be unset, or set to number of CPU cores
echo $GOMAXPROCS
```

**Check for memory pressure causing GC pauses:**

Add `GODEBUG=gctrace=1` to see GC activity:

```bash
GODEBUG=gctrace=1 ./indexer
```

---

## PostgreSQL Connection Refused

### Symptom

```text
failed to connect to database: dial tcp 127.0.0.1:5432: connect: connection refused
```

### Fixes

**Check if PostgreSQL container is running:**

```bash
docker compose ps
docker compose logs postgres
```

**Restart if needed:**

```bash
docker compose restart postgres
```

**Check connection string:**

```yaml
database_url: "postgres://user:password@localhost:5432/btcindex?sslmode=disable"
```

Verify credentials match `docker-compose.yml`.

---

## Bitcoin Core RPC Connection Refused

### Symptom

```text
RPC error: dial tcp 127.0.0.1:8332: connect: connection refused
```

### Fixes

**Check if `bitcoind` is running:**

```bash
ps aux | grep bitcoind
bitcoin-cli getblockcount
```

**Check `bitcoin.conf` has `server=1`:**

```conf
server=1
rpcuser=youruser
rpcpassword=yourpassword
rpcbind=127.0.0.1
rpcallowip=127.0.0.1
```

**Restart `bitcoind` after config changes:**

```bash
bitcoin-cli stop
bitcoind -daemon
```

---

## RPC Authentication Error

### Symptom

```text
RPC error: 401 Unauthorized
```

### Fix

Verify `rpc_url` credentials match `bitcoin.conf`:

```yaml
rpc_url: "http://rpcuser:rpcpassword@127.0.0.1:8332"
```

```conf
rpcuser=rpcuser
rpcpassword=rpcpassword
```

---

## Duplicate Key Violation

### Symptom

```text
ERROR: duplicate key value violates unique constraint "blocks_pkey"
```

### Cause

The indexer is trying to write a block that already exists. This can happen if `START_HEIGHT` is set to a height that has already been indexed.

### Fix

Let the indexer resume automatically from the last indexed height:

```yaml
# Remove start_height override
# The indexer reads the max indexed height from PostgreSQL automatically
```

Or check and set explicitly:

```sql
SELECT MAX(height) FROM blocks;
```

Then set `START_HEIGHT` to one above that value.

---

## `work queue depth exceeded`

### Symptom

```text
RPC error: 429 Work queue depth exceeded
```

### Cause

Bitcoin Core's RPC work queue is full. Too many concurrent requests are being sent.

### Fix

```conf
# bitcoin.conf
rpcworkqueue=128
rpcthreads=16
```

Also reduce indexer workers:

```yaml
workers: 2
batch_size: 2
```

---

## Migration Errors

### Symptom

```text
error: Dirty database version 3. Fix and force version.
```

### Fix

```bash
migrate \
  -path migrations \
  -database "postgres://user:password@localhost:5432/btcindex?sslmode=disable" \
  force 3

migrate \
  -path migrations \
  -database "postgres://user:password@localhost:5432/btcindex?sslmode=disable" \
  up
```

---

## Indexer Stops Without Error

### Possible Causes

- Bitcoin Core stopped (check `bitcoin-cli getblockcount`)
- PostgreSQL connection dropped (check `docker compose ps`)
- The indexer caught up to `safe_tip` and is waiting for new blocks
- OOM kill (check `dmesg | grep -i oom`)

### Check last log line:

```bash
journalctl -u bitcoin-indexer -n 50
```

If the last line is:

```text
Bitcoin node syncing | blocks=886457 headers=948409 remaining=0
```

The indexer has caught up and is in live sync mode — this is normal.

---

## Related Pages

- [Performance](performance.md) — bottleneck tuning
- [Configuration](configuration.md) — all config options
- [Installation](installation.md) — full setup guide
