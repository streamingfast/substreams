CREATE TABLE IF NOT EXISTS moonbird_approvals (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "approved" VARCHAR(40),
    "owner" VARCHAR(40),
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS moonbird_approval_for_alls (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "approved" BOOL,
    "operator" VARCHAR(40),
    "owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS moonbird_expelleds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS moonbird_nesteds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS moonbird_ownership_transferreds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "new_owner" VARCHAR(40),
    "previous_owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS moonbird_pauseds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "account" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS moonbird_refunds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "amount" DECIMAL,
    "buyer" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS moonbird_revenues (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "amount" DECIMAL,
    "beneficiary" VARCHAR(40),
    "num_purchased" DECIMAL
);
CREATE TABLE IF NOT EXISTS moonbird_role_admin_changeds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "new_admin_role" TEXT,
    "previous_admin_role" TEXT,
    "role" TEXT
);
CREATE TABLE IF NOT EXISTS moonbird_role_granteds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "account" VARCHAR(40),
    "role" TEXT,
    "sender" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS moonbird_role_revokeds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "account" VARCHAR(40),
    "role" TEXT,
    "sender" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS moonbird_transfers (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "from" VARCHAR(40),
    "to" VARCHAR(40),
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS moonbird_unnesteds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS moonbird_unpauseds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "account" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS bayc_approvals (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "approved" VARCHAR(40),
    "owner" VARCHAR(40),
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS bayc_approval_for_alls (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "approved" BOOL,
    "operator" VARCHAR(40),
    "owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS bayc_ownership_transferreds (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "new_owner" VARCHAR(40),
    "previous_owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS bayc_transfers (
    "evt_tx_hash" VARCHAR(64),
    "evt_index" INT,
    "evt_block_time" TIMESTAMP,
    "evt_block_number" DECIMAL,
    "from" VARCHAR(40),
    "to" VARCHAR(40),
    "token_id" DECIMAL
);
