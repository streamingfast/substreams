CREATE TABLE IF NOT EXISTS fiouu_pair_created (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "pair" VARCHAR(40),
    "param3" DECIMAL,
    "token0" VARCHAR(40),
    "token1" VARCHAR(40),
    PRIMARY KEY(evt_tx_hash,evt_index)
);
  
