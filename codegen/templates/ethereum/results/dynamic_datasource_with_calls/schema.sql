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
CREATE TABLE IF NOT EXISTS factory_call_create_pool (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "fee" INT,
    "output_pool" VARCHAR(40),
    "token_a" VARCHAR(40),
    "token_b" VARCHAR(40),
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS factory_call_enable_fee_amount (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "fee" INT,
    "tick_spacing" INT,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS factory_call_set_owner (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "u_owner" VARCHAR(40),
    PRIMARY KEY(call_tx_hash,call_ordinal)
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
);CREATE TABLE IF NOT EXISTS pool_call_burn (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount" DECIMAL,
    "output_amount0" DECIMAL,
    "output_amount1" DECIMAL,
    "tick_lower" INT,
    "tick_upper" INT,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_collect (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount0_requested" DECIMAL,
    "amount1_requested" DECIMAL,
    "output_amount0" DECIMAL,
    "output_amount1" DECIMAL,
    "recipient" VARCHAR(40),
    "tick_lower" INT,
    "tick_upper" INT,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_collect_protocol (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount0_requested" DECIMAL,
    "amount1_requested" DECIMAL,
    "output_amount0" DECIMAL,
    "output_amount1" DECIMAL,
    "recipient" VARCHAR(40),
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_flash (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount0" DECIMAL,
    "amount1" DECIMAL,
    "data" TEXT,
    "recipient" VARCHAR(40),
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_increase_observation_cardinality_next (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "observation_cardinality_next" INT,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_initialize (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "sqrt_price_x96" DECIMAL,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_mint (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount" DECIMAL,
    "data" TEXT,
    "output_amount0" DECIMAL,
    "output_amount1" DECIMAL,
    "recipient" VARCHAR(40),
    "tick_lower" INT,
    "tick_upper" INT,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_set_fee_protocol (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "fee_protocol0" INT,
    "fee_protocol1" INT,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);
CREATE TABLE IF NOT EXISTS pool_call_swap (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" DECIMAL,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount_specified" DECIMAL,
    "data" TEXT,
    "output_amount0" DECIMAL,
    "output_amount1" DECIMAL,
    "recipient" VARCHAR(40),
    "sqrt_price_limit_x96" DECIMAL,
    "zero_for_one" BOOL,
    PRIMARY KEY(call_tx_hash,call_ordinal)
);


