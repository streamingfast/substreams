
CREATE TABLE IF NOT EXISTS approvals (
    "trx_hash" VARCHAR(40),
    "log_index" INT,
    "approved" VARCHAR(40),
    "owner" VARCHAR(40),
    "token_id" DECIMAL
);
CREATE TABLE IF NOT EXISTS approval_for_alls (
    "trx_hash" VARCHAR(40),
    "log_index" INT,
    "approved" BOOL,
    "operator" VARCHAR(40),
    "owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS ownership_transferreds (
    "trx_hash" VARCHAR(40),
    "log_index" INT,
    "new_owner" VARCHAR(40),
    "previous_owner" VARCHAR(40)
);
CREATE TABLE IF NOT EXISTS transfers (
    "trx_hash" VARCHAR(40),
    "log_index" INT,
    "from" VARCHAR(40),
    "to" VARCHAR(40),
    "token_id" DECIMAL
);