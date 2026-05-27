CREATE INDEX CONCURRENTLY idx_tx_outputs_0_100k_bh    ON tx_outputs_0_100k(block_height);                                                                 
CREATE INDEX CONCURRENTLY idx_tx_outputs_100k_200k_bh ON tx_outputs_100k_200k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_200k_300k_bh ON tx_outputs_200k_300k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_300k_400k_bh ON tx_outputs_300k_400k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_400k_500k_bh ON tx_outputs_400k_500k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_500k_600k_bh ON tx_outputs_500k_600k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_600k_700k_bh ON tx_outputs_600k_700k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_700k_800k_bh ON tx_outputs_700k_800k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_800k_900k_bh ON tx_outputs_800k_900k(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_900k_1m_bh   ON tx_outputs_900k_1m(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_1m_11m_bh    ON tx_outputs_1m_11m(block_height);
CREATE INDEX CONCURRENTLY idx_tx_outputs_default_bh   ON tx_outputs_default(block_height);

-- spent_height index (for sender backfill - Step 2, coming up next)
CREATE INDEX CONCURRENTLY idx_tx_outputs_0_100k_sh    ON tx_outputs_0_100k(spent_height)    WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_100k_200k_sh ON tx_outputs_100k_200k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_200k_300k_sh ON tx_outputs_200k_300k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_300k_400k_sh ON tx_outputs_300k_400k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_400k_500k_sh ON tx_outputs_400k_500k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_500k_600k_sh ON tx_outputs_500k_600k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_600k_700k_sh ON tx_outputs_600k_700k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_700k_800k_sh ON tx_outputs_700k_800k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_800k_900k_sh ON tx_outputs_800k_900k(spent_height) WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_900k_1m_sh   ON tx_outputs_900k_1m(spent_height)   WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_1m_11m_sh    ON tx_outputs_1m_11m(spent_height)    WHERE is_spent = TRUE;
CREATE INDEX CONCURRENTLY idx_tx_outputs_default_sh   ON tx_outputs_default(spent_height)   WHERE is_spent = TRUE;
