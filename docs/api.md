# API Reference

> HTTP API for querying indexed Bitcoin blockchain data. Built with Go and Gin, backed by PostgreSQL 16.

---

## Overview

The Bitcoin Indexer API is a read-only HTTP service that queries the indexed PostgreSQL tables. Start the API server:

```bash
go run ./cmd/api
```

Default base URL: `http://localhost:<port>/btc`

All responses are JSON. All routes are prefixed with `/btc`.

---

## Endpoints

### `GET /btc/address/:address`

Returns summary information and transaction history for a Bitcoin address.

**Path Parameters:**

| Name | Type | Description |
|---|---|---|
| `address` | string | Bitcoin address |

**Query Parameters:**

| Name | Type | Default | Description |
|---|---|---|---|
| `limit` | int | 100 | Max transactions to return |
| `offset` | int | 0 | Pagination offset |
| `direction` | string | _(all)_ | Filter by `in` (received) or `out` (sent) |
| `role` | string | _(all)_ | Alias for `direction` (backwards-compat) |

**Example:**

```bash
curl "http://localhost:8080/btc/address/bc1qxxxxxxx?limit=10&direction=in"
```

**Response:**

```json
{
  "info": {
    "address": "bc1qxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    "balance_sats": 1500000,
    "tx_count": 12,
    "first_seen_height": 700000,
    "last_seen_height": 886100
  },
  "transactions": [
    {
      "txid": "abc123...",
      "block_height": 886100,
      "timestamp": "2024-03-15T10:22:00Z",
      "value_sats": 1000000,
      "is_input": false,
      "is_output": true
    }
  ]
}
```

**Error Responses:**

| Code | Reason |
|---|---|
| 400 | `address` param missing |
| 404 | Address not found in index |
| 500 | Database error |

---

### `GET /btc/tx/:txid`

Returns full transaction detail including inputs and outputs.

**Path Parameters:**

| Name | Type | Description |
|---|---|---|
| `txid` | string | Transaction ID (hex) |

**Example:**

```bash
curl http://localhost:8080/btc/tx/abc123def456...
```

**Response:**

```json
{
  "txid": "abc123def456...",
  "block_height": 886100,
  "block_hash": "00000000...",
  "timestamp": "2024-03-15T10:22:00Z",
  "version": 2,
  "size": 225,
  "vsize": 141,
  "weight": 561,
  "fee_sats": 1400,
  "is_coinbase": false,
  "inputs": [
    {
      "vin_index": 0,
      "prev_txid": "prev123...",
      "prev_vout": 1,
      "sequence": 4294967295
    }
  ],
  "outputs": [
    {
      "vout_index": 0,
      "value_sats": 50000,
      "address": "bc1qrecipient...",
      "script_type": "p2wpkh"
    }
  ]
}
```

**Error Responses:**

| Code | Reason |
|---|---|
| 400 | `txid` param missing |
| 404 | Transaction not found in index |
| 500 | Database error |

---

### `GET /btc/tx/:txid/trace`

Traces transaction descendants using Apache AGE graph traversal. Returns all transactions reachable from the outputs of the given transaction.

**Path Parameters:**

| Name | Type | Description |
|---|---|---|
| `txid` | string | Transaction ID to trace from |

**Example:**

```bash
curl http://localhost:8080/btc/tx/abc123def456.../trace
```

**Response:**

```json
{
  "txid": "abc123def456...",
  "descendants": [
    {
      "txid": "child111...",
      "depth": 1
    },
    {
      "txid": "grandchild222...",
      "depth": 2
    }
  ]
}
```

**Error Responses:**

| Code | Reason |
|---|---|
| 400 | `txid` param missing |
| 500 | Graph query error |

---

## Error Format

All errors return a consistent JSON body:

```json
{
  "error": "description of what went wrong"
}
```

---

## Running the API

```bash
# Run directly
go run ./cmd/api

# Build and run
go build -o api ./cmd/api
./api
```

Port is set via `config.yaml` or the environment:

```bash
export PORT=8080
```

---

## Related Pages

- [Schema](schema.md) — underlying table structure the API reads from
- [Installation](installation.md) — full setup guide
- [Architecture](architecture.md) — how data is indexed before the API reads it
