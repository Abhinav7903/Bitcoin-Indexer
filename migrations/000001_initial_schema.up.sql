-- Extensions
CREATE EXTENSION IF NOT EXISTS age CASCADE;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

-- ============================================================
-- blocks
-- ============================================================
CREATE TABLE blocks (
    height         INT          PRIMARY KEY,
    hash           BYTEA        NOT NULL UNIQUE,
    prev_hash      BYTEA        NOT NULL,
    merkle_root    BYTEA        NOT NULL,
    block_time     TIMESTAMPTZ  NOT NULL,
    bits           BIGINT       NOT NULL,
    nonce          BIGINT       NOT NULL,
    version        INT          NOT NULL,
    tx_count       INT          NOT NULL DEFAULT 0,
    size_bytes     INT          NOT NULL,
    weight         INT          NOT NULL,
    total_fees_sats BIGINT      NOT NULL DEFAULT 0,
    indexed_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ============================================================
-- transactions  (partitioned by block_height)
-- FIX: PRIMARY KEY must include the partition key (block_height)
-- ============================================================
CREATE TABLE transactions (
    txid          BYTEA     NOT NULL,
    block_height  INT       NOT NULL,   -- partition key
    block_hash    BYTEA     NOT NULL,
    tx_index      INT       NOT NULL,
    version       INT       NOT NULL,
    locktime      BIGINT    NOT NULL,
    is_coinbase   BOOL      NOT NULL DEFAULT FALSE,
    input_count   SMALLINT  NOT NULL DEFAULT 0,
    output_count  SMALLINT  NOT NULL DEFAULT 0,
    fee_sats      BIGINT,
    size_bytes    INT,
    vsize         INT,
    weight        INT,
    has_segwit    BOOL      NOT NULL DEFAULT FALSE,
    PRIMARY KEY (block_height, txid)   -- block_height MUST be first or included
) PARTITION BY RANGE (block_height);

-- ============================================================
-- tx_outputs  (partitioned by block_height)
-- FIX: PRIMARY KEY must include block_height
-- ============================================================
CREATE TABLE tx_outputs (
    txid           BYTEA     NOT NULL,
    vout_idx       INT       NOT NULL,
    block_height   INT       NOT NULL,   -- partition key
    value_sats     BIGINT    NOT NULL,
    script_pubkey  BYTEA,
    script_type    SMALLINT  NOT NULL DEFAULT 7,
    address        TEXT,
    is_spent       BOOL      NOT NULL DEFAULT FALSE,
    spending_txid  BYTEA,
    spending_vin   INT,
    spent_height   INT,
    PRIMARY KEY (block_height, txid, vout_idx)  -- block_height included
) PARTITION BY RANGE (block_height);

-- ============================================================
-- tx_inputs  (partitioned by block_height)
-- FIX: PRIMARY KEY must include block_height
-- ============================================================
CREATE TABLE tx_inputs (
    txid         BYTEA     NOT NULL,
    vin_idx      INT       NOT NULL,
    block_height INT       NOT NULL,   -- partition key
    prev_txid    BYTEA,
    prev_vout    INT,
    script_sig   BYTEA,
    witness_data BYTEA[],
    sequence_no  BIGINT    NOT NULL DEFAULT 4294967294,
    PRIMARY KEY (block_height, txid, vin_idx)  -- block_height included
) PARTITION BY RANGE (block_height);

-- ============================================================
-- address_transactions  (partitioned by block_height)
-- FIX 1: PRIMARY KEY must include block_height
-- FIX 2: Added block_time column (writer.go inserts it)
-- ============================================================
CREATE TABLE address_transactions (
    address        TEXT        NOT NULL,
    block_height   INT         NOT NULL,   -- partition key
    tx_index       INT         NOT NULL,
    txid           BYTEA       NOT NULL,
    role           SMALLINT    NOT NULL DEFAULT 0,  -- 0=receiver,1=sender
    net_value_sats BIGINT      NOT NULL DEFAULT 0,
    block_time     TIMESTAMPTZ NOT NULL,             -- FIX: was missing
    PRIMARY KEY (address, block_height, tx_index, txid, role)
) PARTITION BY RANGE (block_height);

-- ============================================================
-- utxo_set  (hot, unpartitioned)
-- ============================================================
CREATE TABLE utxo_set (
    txid         BYTEA    NOT NULL,
    vout_idx     INT      NOT NULL,
    address      TEXT     NOT NULL,
    value_sats   BIGINT   NOT NULL,
    block_height INT      NOT NULL,
    script_type  SMALLINT NOT NULL DEFAULT 7,
    PRIMARY KEY (txid, vout_idx)
);

-- ============================================================
-- address_balances  (cached, unpartitioned)
-- ============================================================
CREATE TABLE address_balances (
    address             TEXT   PRIMARY KEY,
    balance_sats        BIGINT NOT NULL DEFAULT 0,
    total_received_sats BIGINT NOT NULL DEFAULT 0,
    total_sent_sats     BIGINT NOT NULL DEFAULT 0,
    utxo_count          INT    NOT NULL DEFAULT 0,
    tx_count            INT    NOT NULL DEFAULT 0,
    first_seen_height   INT,
    last_seen_height    INT,
    updated_at_height   INT    NOT NULL DEFAULT 0
);

-- ============================================================
-- index_state  (single-row checkpoint)
-- ============================================================
CREATE TABLE index_state (
    id                  INT         PRIMARY KEY DEFAULT 1,
    last_indexed_height INT         NOT NULL DEFAULT 0,
    last_indexed_hash   BYTEA,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT single_index_state_row CHECK (id = 1)
);

-- ============================================================
-- Partitions: transactions
-- ============================================================
CREATE TABLE transactions_0_100k   PARTITION OF transactions FOR VALUES FROM (0)       TO (100000);
CREATE TABLE transactions_100k_200k PARTITION OF transactions FOR VALUES FROM (100000)  TO (200000);
CREATE TABLE transactions_200k_300k PARTITION OF transactions FOR VALUES FROM (200000)  TO (300000);
CREATE TABLE transactions_300k_400k PARTITION OF transactions FOR VALUES FROM (300000)  TO (400000);
CREATE TABLE transactions_400k_500k PARTITION OF transactions FOR VALUES FROM (400000)  TO (500000);
CREATE TABLE transactions_500k_600k PARTITION OF transactions FOR VALUES FROM (500000)  TO (600000);
CREATE TABLE transactions_600k_700k PARTITION OF transactions FOR VALUES FROM (600000)  TO (700000);
CREATE TABLE transactions_700k_800k PARTITION OF transactions FOR VALUES FROM (700000)  TO (800000);
CREATE TABLE transactions_800k_900k PARTITION OF transactions FOR VALUES FROM (800000)  TO (900000);
CREATE TABLE transactions_900k_1m   PARTITION OF transactions FOR VALUES FROM (900000)  TO (1000000);
CREATE TABLE transactions_1m_11m    PARTITION OF transactions FOR VALUES FROM (1000000) TO (1100000);
CREATE TABLE transactions_default   PARTITION OF transactions DEFAULT;

-- ============================================================
-- Partitions: tx_outputs
-- FIX: Added DEFAULT partition (was missing)
-- ============================================================
CREATE TABLE tx_outputs_0_100k   PARTITION OF tx_outputs FOR VALUES FROM (0)       TO (100000);
CREATE TABLE tx_outputs_100k_200k PARTITION OF tx_outputs FOR VALUES FROM (100000)  TO (200000);
CREATE TABLE tx_outputs_200k_300k PARTITION OF tx_outputs FOR VALUES FROM (200000)  TO (300000);
CREATE TABLE tx_outputs_300k_400k PARTITION OF tx_outputs FOR VALUES FROM (300000)  TO (400000);
CREATE TABLE tx_outputs_400k_500k PARTITION OF tx_outputs FOR VALUES FROM (400000)  TO (500000);
CREATE TABLE tx_outputs_500k_600k PARTITION OF tx_outputs FOR VALUES FROM (500000)  TO (600000);
CREATE TABLE tx_outputs_600k_700k PARTITION OF tx_outputs FOR VALUES FROM (600000)  TO (700000);
CREATE TABLE tx_outputs_700k_800k PARTITION OF tx_outputs FOR VALUES FROM (700000)  TO (800000);
CREATE TABLE tx_outputs_800k_900k PARTITION OF tx_outputs FOR VALUES FROM (800000)  TO (900000);
CREATE TABLE tx_outputs_900k_1m   PARTITION OF tx_outputs FOR VALUES FROM (900000)  TO (1000000);
CREATE TABLE tx_outputs_1m_11m    PARTITION OF tx_outputs FOR VALUES FROM (1000000) TO (1100000);
CREATE TABLE tx_outputs_default   PARTITION OF tx_outputs DEFAULT;  -- FIX: added

-- ============================================================
-- Partitions: tx_inputs
-- FIX: Added DEFAULT partition (was missing)
-- ============================================================
CREATE TABLE tx_inputs_0_100k   PARTITION OF tx_inputs FOR VALUES FROM (0)       TO (100000);
CREATE TABLE tx_inputs_100k_200k PARTITION OF tx_inputs FOR VALUES FROM (100000)  TO (200000);
CREATE TABLE tx_inputs_200k_300k PARTITION OF tx_inputs FOR VALUES FROM (200000)  TO (300000);
CREATE TABLE tx_inputs_300k_400k PARTITION OF tx_inputs FOR VALUES FROM (300000)  TO (400000);
CREATE TABLE tx_inputs_400k_500k PARTITION OF tx_inputs FOR VALUES FROM (400000)  TO (500000);
CREATE TABLE tx_inputs_500k_600k PARTITION OF tx_inputs FOR VALUES FROM (500000)  TO (600000);
CREATE TABLE tx_inputs_600k_700k PARTITION OF tx_inputs FOR VALUES FROM (600000)  TO (700000);
CREATE TABLE tx_inputs_700k_800k PARTITION OF tx_inputs FOR VALUES FROM (700000)  TO (800000);
CREATE TABLE tx_inputs_800k_900k PARTITION OF tx_inputs FOR VALUES FROM (800000)  TO (900000);
CREATE TABLE tx_inputs_900k_1m   PARTITION OF tx_inputs FOR VALUES FROM (900000)  TO (1000000);
CREATE TABLE tx_inputs_1m_11m    PARTITION OF tx_inputs FOR VALUES FROM (1000000) TO (1100000);
CREATE TABLE tx_inputs_default   PARTITION OF tx_inputs DEFAULT;  -- FIX: added

-- ============================================================
-- Partitions: address_transactions
-- FIX: Added DEFAULT partition (was missing)
-- ============================================================
CREATE TABLE address_tx_0_100k   PARTITION OF address_transactions FOR VALUES FROM (0)       TO (100000);
CREATE TABLE address_tx_100k_200k PARTITION OF address_transactions FOR VALUES FROM (100000)  TO (200000);
CREATE TABLE address_tx_200k_300k PARTITION OF address_transactions FOR VALUES FROM (200000)  TO (300000);
CREATE TABLE address_tx_300k_400k PARTITION OF address_transactions FOR VALUES FROM (300000)  TO (400000);
CREATE TABLE address_tx_400k_500k PARTITION OF address_transactions FOR VALUES FROM (400000)  TO (500000);
CREATE TABLE address_tx_500k_600k PARTITION OF address_transactions FOR VALUES FROM (500000)  TO (600000);
CREATE TABLE address_tx_600k_700k PARTITION OF address_transactions FOR VALUES FROM (600000)  TO (700000);
CREATE TABLE address_tx_700k_800k PARTITION OF address_transactions FOR VALUES FROM (700000)  TO (800000);
CREATE TABLE address_tx_800k_900k PARTITION OF address_transactions FOR VALUES FROM (800000)  TO (900000);
CREATE TABLE address_tx_900k_1m   PARTITION OF address_transactions FOR VALUES FROM (900000)  TO (1000000);
CREATE TABLE address_tx_1m_11m    PARTITION OF address_transactions FOR VALUES FROM (1000000) TO (1100000);
CREATE TABLE address_tx_default   PARTITION OF address_transactions DEFAULT;  -- FIX: added

-- ============================================================
-- Indexes: blocks
-- ============================================================
CREATE INDEX idx_blocks_hash        ON blocks (hash);
CREATE INDEX idx_blocks_height_brin ON blocks USING BRIN (height);
CREATE INDEX idx_blocks_time_brin   ON blocks USING BRIN (block_time);

-- ============================================================
-- Indexes: transactions
-- ============================================================
CREATE INDEX idx_tx_txid        ON transactions (txid);
CREATE INDEX idx_tx_height_brin ON transactions USING BRIN (block_height) WITH (pages_per_range = 32);
CREATE INDEX idx_tx_coinbase    ON transactions (block_height) WHERE is_coinbase = TRUE;

-- ============================================================
-- Indexes: tx_outputs
-- ============================================================
CREATE INDEX idx_txout_txid        ON tx_outputs (txid, vout_idx);
CREATE INDEX idx_txout_height_brin ON tx_outputs USING BRIN (block_height) WITH (pages_per_range = 32);
CREATE INDEX idx_txout_addr_unspent ON tx_outputs (address, value_sats DESC)
    WHERE is_spent = FALSE AND address IS NOT NULL;
CREATE INDEX idx_txout_spending    ON tx_outputs (spending_txid) WHERE spending_txid IS NOT NULL;

-- ============================================================
-- Indexes: tx_inputs
-- ============================================================
CREATE INDEX idx_txin_txid        ON tx_inputs (txid);
CREATE INDEX idx_txin_prev        ON tx_inputs (prev_txid, prev_vout) WHERE prev_txid IS NOT NULL;
CREATE INDEX idx_txin_height_brin ON tx_inputs USING BRIN (block_height) WITH (pages_per_range = 32);

-- ============================================================
-- Indexes: address_transactions
-- ============================================================
CREATE INDEX idx_addrtx_addr        ON address_transactions (address, block_height DESC, tx_index DESC);
CREATE INDEX idx_addrtx_height_brin ON address_transactions USING BRIN (block_height) WITH (pages_per_range = 32);

-- ============================================================
-- Indexes: utxo_set
-- ============================================================
CREATE INDEX idx_utxo_address   ON utxo_set (address, value_sats DESC);
CREATE INDEX idx_utxo_height_brin ON utxo_set USING BRIN (block_height) WITH (pages_per_range = 32);

-- ============================================================
-- Seed index_state
-- ============================================================
INSERT INTO index_state (id, last_indexed_height) VALUES (1, 0)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Apache AGE graph (optional — comment out if not using AGE)
-- ============================================================
SET search_path = ag_catalog, "$user", public;

SELECT create_graph('bitcoin');

SELECT create_vlabel('bitcoin', 'block');
SELECT create_vlabel('bitcoin', 'transaction');
SELECT create_vlabel('bitcoin', 'address');

SELECT create_elabel('bitcoin', 'spends');
SELECT create_elabel('bitcoin', 'sends');

-- ============================================================
-- Comments
-- ============================================================
COMMENT ON TABLE blocks IS 'One row per Bitcoin block.';
COMMENT ON TABLE transactions IS 'All transactions, partitioned by block_height. PK includes block_height (required for partitioned tables).';
COMMENT ON TABLE tx_outputs IS 'All transaction outputs with spend status. PK includes block_height.';
COMMENT ON TABLE tx_inputs IS 'All transaction inputs; join to tx_outputs via prev_txid and prev_vout.';
COMMENT ON TABLE address_transactions IS 'Denormalized address history. Zero-join address tx history queries. Includes block_time.';
COMMENT ON TABLE utxo_set IS 'Current live UTXO set. Delete spent rows and insert new rows per batch.';
COMMENT ON TABLE address_balances IS 'Cached address balances updated after each ingested batch.';