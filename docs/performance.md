# Performance

> Benchmarks, bottleneck analysis, and tuning guide for Bitcoin Indexer.

---

## Understanding Indexing Speed

Bitcoin Indexer speed is **not constant** across the chain. Early Bitcoin blocks (2009–2013) had very few transactions — often under 10 per block. Recent blocks (2023+) regularly contain 3,000–5,000 transactions. The same hardware will index early blocks 30–50x faster than recent blocks.

### Approximate Speed by Era

| Block Range | Era | Avg txs/block | Approximate Speed |
|---|---|---|---|
| 0 – 200,000 | 2009–2012 | 5–50 | 8–15 blocks/sec |
| 200,000 – 400,000 | 2012–2015 | 50–500 | 2–5 blocks/sec |
| 400,000 – 600,000 | 2015–2018 | 500–2,000 | 0.3–1 block/sec |
| 600,000 – 750,000 | 2018–2021 | 1,500–2,500 | 0.2–0.5 block/sec |
| 750,000 – 900,000 | 2021–2024 | 2,500–4,000 | 0.1–0.3 block/sec |
| 900,000+ | 2024+ | 3,000–5,000+ | 0.05–0.2 block/sec |

> These numbers assume a fully synced Bitcoin Core node on NVMe storage. During IBD, `getblockhash` latency spikes can reduce all numbers significantly.

### Estimated Total Sync Time

Starting from block 0 with a fully synced node on modern hardware:

- Blocks 0 – 400,000: **~2–4 hours**
- Blocks 400,000 – 700,000: **~8–16 hours**
- Blocks 700,000 – 950,000: **~24–48 hours**
- **Total: approximately 2–4 days**

If your Bitcoin Core node is still in IBD while indexing, add 50–100% to the above estimates due to RPC stall time.

---

## Reading the Timing Logs

The indexer emits per-stage timing for every block:

```text
Block 141045: RPC total=872ms getblockhash=830ms getblock=41ms parse=45µs (6 txs)
Batch 141045-141046: fetched 2 blocks (87 txs) in 876ms wall time, DB write in 34ms
```

Each component tells you something different:

```
getblockhash  = time waiting for Bitcoin Core to return the block hash
getblock      = time to fetch the full decoded block JSON
parse         = time to convert JSON into Go structs and DB rows
DB write      = time for PostgreSQL COPY batch write
wall time     = total elapsed time for the slowest worker in the batch
```

**Batch wall time is the slowest worker, not the sum.** Workers run concurrently.

---

## Bottleneck Identification

### Slow `getblockhash`

```text
getblockhash=8s getblock=20ms parse=1ms
```

**Cause:** Bitcoin Core RPC is stalling. Most common during IBD when Bitcoin Core is busy with:
- LevelDB compaction
- Block validation
- UTXO set writes
- RPC queue pressure

**Fix:**
- Increase `dbcache` in `bitcoin.conf`
- Increase `rpcthreads` and `rpcworkqueue`
- Wait for IBD to complete — latency improves substantially after the node is synced

---

### Slow `getblock`

```text
getblockhash=5ms getblock=6s parse=1ms
```

**Cause:** Verbose block fetch (`verbosity=2`) is slow. Common with:
- Very large recent blocks (3MB+)
- Slow disk on the Bitcoin Core host
- Bitcoin Core CPU overload
- Too many concurrent RPC workers competing

**Fix:**
- Ensure Bitcoin Core storage is on SSD/NVMe
- Reduce `workers` to lower RPC concurrency
- Check Bitcoin Core CPU and disk I/O: `top`, `iostat`

---

### Slow `parse`

```text
getblockhash=5ms getblock=30ms parse=2s
```

**Cause:** Go CPU-bound. JSON decoding or struct allocation is taking too long.

**Fix:**
- Check Go process CPU with `top`
- Look for memory pressure causing GC pauses
- Check if `GOMAXPROCS` is set correctly (should be unset or match CPU count)

---

### Slow DB Write

```text
DB write in 5s
```

**Cause:** PostgreSQL write is saturated.

**Fix:**
- Check PostgreSQL disk I/O: `iostat -x 1`
- Check for table bloat or missing partitions in `pg_stat_user_tables`
- Verify partition exists for the current block height range
- Check WAL settings: consider `synchronous_commit=off` during initial sync
- Increase `shared_buffers` and `checkpoint_completion_target`

---

## Worker and Batch Size Tuning

The relationship between `workers` and `batch_size` is critical:

```
workers = batch_size = N

N blocks fetched concurrently → N parsed in memory → 1 COPY write to PostgreSQL
```

Mismatching these is a common mistake:

| Config | Result |
|---|---|
| `workers=2, batch_size=2` | ✅ Correct. Both workers busy. |
| `workers=8, batch_size=2` | ❌ 6 workers idle. No throughput gain. |
| `workers=2, batch_size=8` | ❌ Only 2 workers fetch 8 blocks. Sequential. |
| `workers=8, batch_size=8` | ✅ Good for post-IBD. High parallelism. |

**During IBD:** keep both at `2` to avoid overloading a busy node.

**After IBD:** try `4/4`, `8/8`, or `16/16` depending on hardware.

---

## Storage Recommendations

Bitcoin Indexer involves heavy disk I/O from two sources: Bitcoin Core and PostgreSQL. Ideally they are on separate disks.

| Component | Recommended Storage | Minimum |
|---|---|---|
| Bitcoin Core (blocks + chainstate) | NVMe SSD | SATA SSD |
| PostgreSQL data directory | NVMe SSD | SATA SSD |
| Separate disks for each | Strongly recommended | — |

Spinning HDD is not recommended for either. `getblock` can take 5–30 seconds per call on HDD for recent large blocks.

---

## PostgreSQL Tuning for Initial Sync

For the initial historical index, these PostgreSQL settings significantly improve write throughput:

```conf
# postgresql.conf
shared_buffers = 4GB               # 25% of RAM
work_mem = 256MB
maintenance_work_mem = 2GB
checkpoint_completion_target = 0.9
wal_buffers = 64MB
max_wal_size = 4GB
synchronous_commit = off           # Async commits, faster writes
```

For maximum speed during initial load only:

```conf
fsync = off                        # ⚠️ DANGER: data loss on crash
wal_level = minimal
```

> ⚠️ Re-enable `fsync=on` and `synchronous_commit=on` immediately after the initial sync is complete.

---

## Bitcoin Core Tuning for Fast IBD

```conf
dbcache=16384       # As large as RAM allows
rpcthreads=8
rpcworkqueue=64
par=8               # More validation threads
```

Restart `bitcoind` after changing `bitcoin.conf`.

Check if the change took effect:

```bash
bitcoin-cli getmemoryinfo
```

---

## Monitoring Indexer Progress

Watch logs for batch rate:

```bash
journalctl -u bitcoin-indexer -f | grep "Batch"
```

Check blocks indexed per second:

```bash
# Count how many batch lines in last 60s
journalctl -u bitcoin-indexer --since "1 minute ago" | grep -c "Batch"
```

Query PostgreSQL for current indexed height:

```sql
SELECT MAX(height) AS indexed_height FROM blocks;
```

Estimate remaining time:

```sql
SELECT
  MAX(height) AS current,
  886000 AS target,
  886000 - MAX(height) AS remaining
FROM blocks;
```

---

## Related Pages

- [Configuration](configuration.md) — `workers`, `batch_size`, `dbcache` reference
- [Troubleshooting](troubleshooting.md) — slow stage diagnosis
- [Architecture](architecture.md) — pipeline and concurrency design
