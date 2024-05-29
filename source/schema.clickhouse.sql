CREATE TABLE IF NOT EXISTS fiouu_pair_created (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "pair" VARCHAR(40),
    "param3" UInt256,
    "token0" VARCHAR(40),
    "token1" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");


