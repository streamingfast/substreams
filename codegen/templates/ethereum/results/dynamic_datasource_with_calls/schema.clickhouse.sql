CREATE TABLE IF NOT EXISTS factory_fee_amount_enabled (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "fee" UInt32,
    "tick_spacing" Int32
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS factory_owner_changed (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "new_owner" VARCHAR(40),
    "old_owner" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS factory_pool_created (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "fee" UInt32,
    "pool" VARCHAR(40),
    "tick_spacing" Int32,
    "token0" VARCHAR(40),
    "token1" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");

CREATE TABLE IF NOT EXISTS factory_call_create_pool (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "fee" UInt32,
    "output_pool" VARCHAR(40),
    "token_a" VARCHAR(40),
    "token_b" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS factory_call_enable_fee_amount (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "fee" UInt32,
    "tick_spacing" Int32
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS factory_call_set_owner (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "u_owner" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_burn (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "amount" UInt128,
    "amount0" UInt256,
    "amount1" UInt256,
    "owner" VARCHAR(40),
    "tick_lower" Int32,
    "tick_upper" Int32
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_collect (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "amount0" UInt128,
    "amount1" UInt128,
    "owner" VARCHAR(40),
    "recipient" VARCHAR(40),
    "tick_lower" Int32,
    "tick_upper" Int32
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_collect_protocol (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "amount0" UInt128,
    "amount1" UInt128,
    "recipient" VARCHAR(40),
    "sender" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_flash (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "amount0" UInt256,
    "amount1" UInt256,
    "paid0" UInt256,
    "paid1" UInt256,
    "recipient" VARCHAR(40),
    "sender" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_increase_observation_cardinality_next (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "observation_cardinality_next_new" UInt16,
    "observation_cardinality_next_old" UInt16
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_initialize (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "sqrt_price_x96" UInt256,
    "tick" Int32
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_mint (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "amount" UInt128,
    "amount0" UInt256,
    "amount1" UInt256,
    "owner" VARCHAR(40),
    "sender" VARCHAR(40),
    "tick_lower" Int32,
    "tick_upper" Int32
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_set_fee_protocol (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "fee_protocol0_new" UInt8,
    "fee_protocol0_old" UInt8,
    "fee_protocol1_new" UInt8,
    "fee_protocol1_old" UInt8
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_swap (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" UInt64,
    "evt_address" VARCHAR(40),
    "amount0" Int256,
    "amount1" Int256,
    "liquidity" UInt128,
    "recipient" VARCHAR(40),
    "sender" VARCHAR(40),
    "sqrt_price_x96" UInt256,
    "tick" Int32
) ENGINE = MergeTree PRIMARY KEY ("evt_tx_hash","evt_index");
CREATE TABLE IF NOT EXISTS pool_call_burn (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount" UInt128,
    "output_amount0" UInt256,
    "output_amount1" UInt256,
    "tick_lower" Int32,
    "tick_upper" Int32
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_collect (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount0_requested" UInt128,
    "amount1_requested" UInt128,
    "output_amount0" UInt128,
    "output_amount1" UInt128,
    "recipient" VARCHAR(40),
    "tick_lower" Int32,
    "tick_upper" Int32
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_collect_protocol (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount0_requested" UInt128,
    "amount1_requested" UInt128,
    "output_amount0" UInt128,
    "output_amount1" UInt128,
    "recipient" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_flash (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount0" UInt256,
    "amount1" UInt256,
    "data" TEXT,
    "recipient" VARCHAR(40)
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_increase_observation_cardinality_next (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "observation_cardinality_next" UInt16
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_initialize (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "sqrt_price_x96" UInt256
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_mint (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount" UInt128,
    "data" TEXT,
    "output_amount0" UInt256,
    "output_amount1" UInt256,
    "recipient" VARCHAR(40),
    "tick_lower" Int32,
    "tick_upper" Int32
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_set_fee_protocol (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "fee_protocol0" UInt8,
    "fee_protocol1" UInt8
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
CREATE TABLE IF NOT EXISTS pool_call_swap (
    "call_tx_hash" VARCHAR(64),
    "call_block_time" TIMESTAMP,
    "call_block_number" UInt64,
    "call_ordinal" INT,
    "call_success" BOOL,
    "call_address" VARCHAR(40),
    "amount_specified" Int256,
    "data" TEXT,
    "output_amount0" Int256,
    "output_amount1" Int256,
    "recipient" VARCHAR(40),
    "sqrt_price_limit_x96" UInt256,
    "zero_for_one" BOOL
) ENGINE = MergeTree PRIMARY KEY ("call_tx_hash","call_ordinal");
