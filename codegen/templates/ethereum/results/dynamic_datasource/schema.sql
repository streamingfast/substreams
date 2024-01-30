CREATE TABLE IF NOT EXISTS factory_fee_amount_enabled (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "fee" INT,
    "tick_spacing" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS factory_owner_changed (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "new_owner" VARCHAR(40),
    "old_owner" VARCHAR(40),
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS factory_pool_created (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "fee" INT,
    "pool" VARCHAR(40),
    "tick_spacing" INT,
    "token0" VARCHAR(40),
    "token1" VARCHAR(40),
    PRIMARY KEY(evt_tx_hash,evt_index)
);

CREATE TABLE IF NOT EXISTS pool_burn (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "amount" DECIMAL,
    "amount0" DECIMAL,
    "amount1" DECIMAL,
    "owner" VARCHAR(40),
    "tick_lower" INT,
    "tick_upper" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_collect (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "amount0" DECIMAL,
    "amount1" DECIMAL,
    "owner" VARCHAR(40),
    "recipient" VARCHAR(40),
    "tick_lower" INT,
    "tick_upper" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_collect_protocol (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "amount0" DECIMAL,
    "amount1" DECIMAL,
    "recipient" VARCHAR(40),
    "sender" VARCHAR(40),
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_flash (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "amount0" DECIMAL,
    "amount1" DECIMAL,
    "paid0" DECIMAL,
    "paid1" DECIMAL,
    "recipient" VARCHAR(40),
    "sender" VARCHAR(40),
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_increase_observation_cardinality_next (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "observation_cardinality_next_new" INT,
    "observation_cardinality_next_old" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_initialize (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "sqrt_price_x96" DECIMAL,
    "tick" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_mint (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "amount" DECIMAL,
    "amount0" DECIMAL,
    "amount1" DECIMAL,
    "owner" VARCHAR(40),
    "sender" VARCHAR(40),
    "tick_lower" INT,
    "tick_upper" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_set_fee_protocol (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "fee_protocol0_new" INT,
    "fee_protocol0_old" INT,
    "fee_protocol1_new" INT,
    "fee_protocol1_old" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
CREATE TABLE IF NOT EXISTS pool_swap (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "evt_address" VARCHAR(40),
    "amount0" DECIMAL,
    "amount1" DECIMAL,
    "liquidity" DECIMAL,
    "recipient" VARCHAR(40),
    "sender" VARCHAR(40),
    "sqrt_price_x96" DECIMAL,
    "tick" INT,
    PRIMARY KEY(evt_tx_hash,evt_index)
);
