
CREATE TABLE IF NOT EXISTS approvals (
    "trx_hash" VARCHAR(64),
    "log_index" INT,
    "timestamp_s" DECIMAL,
    "block_num" DECIMAL,
    "approved" VARCHAR(40),
    "owner" VARCHAR(40),
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS approval_for_alls (
    "trx_hash" VARCHAR(64),
    "log_index" INT,
    "timestamp_s" DECIMAL,
    "block_num" DECIMAL,
    "approved" BOOL,
    "operator" VARCHAR(40),
    "owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS ownership_transferreds (
    "trx_hash" VARCHAR(64),
    "log_index" INT,
    "timestamp_s" DECIMAL,
    "block_num" DECIMAL,
    "new_owner" VARCHAR(40),
    "previous_owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS transfers (
    "trx_hash" VARCHAR(64),
    "log_index" INT,
    "timestamp_s" DECIMAL,
    "block_num" DECIMAL,
    "from" VARCHAR(40),
    "to" VARCHAR(40),
    "token_id" DECIMAL
);