# Re-org Handling Reference

This document describes how `substreams-sink-sql` implements re-organization (re-org) handling to maintain data consistency when blockchain reorganizations occur.

## Overview

Blockchain networks can experience reorganizations where previously accepted blocks are replaced by a different chain. When this happens, database changes that were based on the replaced blocks must be reverted to maintain consistency with the canonical chain.

The sink implements different re-org handling strategies depending on the data processing model used:

- **DatabaseChanges Model** (`db_out` modules) - Tracks individual operations in a history table
- **Relational Mappings** (from Protobuf directly) - _Documentation to come soon_
- **Delayed block signalling** (`undo-buffer` flags) - _Documentation to come soon_

This document currently focuses on the **DatabaseChanges model** implementation.

## DatabaseChanges Model Re-org Handling

The DatabaseChanges model (used by `db_out` modules) handles re-orgs through a four-phase process:

1. **Tracking changes** - Storing a record of all database operations in a history table, only in the reversible segment of the chain, this means there is no operations happening when backfilling historical segments.
2. **Detecting re-orgs** - Receiving undo signals when reorganizations occur
3. **Reverting changes** - Rolling back operations from forked blocks
4. **Cleaning up** - Removing history records for finalized blocks

### History Table Schema

The `substreams_history` table tracks all database operations with the following structure:

```sql
CREATE TABLE substreams_history (
    id           SERIAL PRIMARY KEY,
    op           char,           -- Operation type: 'I' (Insert), 'U' (Update), 'D' (Delete)
    table_name   text,           -- Full table name including schema
    pk           text,           -- Primary key as JSON object
    prev_value   text,           -- Previous row data as JSON (for updates/deletes)
    block_num    bigint          -- Block number when operation occurred
);
```

### Example Walkthrough

Let's trace through a complete re-org scenario using a simple `transfer` table with the DatabaseChanges model:

```sql
CREATE TABLE transfer (
    id     varchar PRIMARY KEY,
    "from" varchar,
    "to"   varchar,
    value  bigint
);
```

#### 1. Normal Operations (Blocks 100-102)

**Block 100**: Insert new transfer
```sql
-- Application query
INSERT INTO transfer (id, "from", "to", value) VALUES ('tx1', 'alice', 'bob', 1000);

-- History tracking (saves initial insert)
INSERT INTO substreams_history (op, table_name, pk, block_num)
VALUES ('I', 'public.transfer', '{"id":"tx1"}', 100);
```

**Block 101**: Update existing transfer
```sql
-- History tracking (saves current state before update)
INSERT INTO substreams_history (op, table_name, pk, prev_value, block_num)
SELECT 'U', 'public.transfer', '{"id":"tx1"}', row_to_json(transfer), 101
FROM transfer WHERE id = 'tx1';

-- Application query
UPDATE transfer SET value = 1500 WHERE id = 'tx1';
```

**Block 102**: Delete transfer
```sql
-- History tracking (saves current state before delete)
INSERT INTO substreams_history (op, table_name, pk, prev_value, block_num)
SELECT 'D', 'public.transfer', '{"id":"tx1"}', row_to_json(transfer), 102
FROM transfer WHERE id = 'tx1';

-- Application query
DELETE FROM transfer WHERE id = 'tx1';
```

#### 2. Re-org Detection

When a re-org is detected at block 101 (meaning blocks 101+ are now invalid), the sink receives an undo signal with `lastValidFinalBlock = 100`.

#### 3. Reversion Process

The sink queries the history table for all operations after the last valid block:

```sql
SELECT op, table_name, pk, prev_value, block_num
FROM substreams_history
WHERE block_num > 100
ORDER BY block_num DESC;
```

Results (processed in reverse chronological order):
```
op | table_name      | pk              | prev_value                                    | block_num
---|-----------------|-----------------|-----------------------------------------------|----------
D  | public.transfer | {"id":"tx1"}    | {"id":"tx1","from":"alice","to":"bob","value":1500} | 102
U  | public.transfer | {"id":"tx1"}    | {"id":"tx1","from":"alice","to":"bob","value":1000} | 101
```

#### 4. Applying Reversions

**Revert Delete (Block 102)**:
```sql
-- Restore the deleted row
INSERT INTO public.transfer
SELECT * FROM json_populate_record(null::public.transfer,
    '{"id":"tx1","from":"alice","to":"bob","value":1500}');
```

**Revert Update (Block 101)**:
```sql
-- Restore previous values
UPDATE public.transfer
SET (id,"from","to",value) = (
    (SELECT id,"from","to",value FROM json_populate_record(null::public.transfer,
        '{"id":"tx1","from":"alice","to":"bob","value":1000}'))
)
WHERE id = 'tx1';
```

#### 5. History Cleanup

After successful reversion, remove invalidated history records:

```sql
DELETE FROM substreams_history WHERE block_num > 100;
```

#### 6. Final State

The database is now consistent with block 100:
- `transfer` table contains: `{id: 'tx1', from: 'alice', to: 'bob', value: 1000}`
- `substreams_history` contains only the insert record from block 100

### Operation Types and Reversions

| Original Operation | Reversion Strategy | Notes |
|-------------------|-------------------|-------|
| **Insert** (`I`) | `DELETE` using primary key | Removes the inserted row |
| **Update** (`U`) | `UPDATE` with previous values | Restores old column values |
| **Delete** (`D`) | `INSERT` with previous values | Recreates the deleted row |
| **Upsert** | Context-dependent | Saved as either `I` or `U` based on whether row existed |

### Finality and Cleanup

When blocks become final (irreversible), their history records are no longer needed:

```sql
-- Remove history for blocks that are now final
DELETE FROM substreams_history WHERE block_num <= {finalBlockNum};
```

This cleanup happens automatically during normal operation to prevent unbounded growth of the history table.

## Limitations

- **Primary keys required**: All tables must have primary keys for re-org handling to work
- **PostgreSQL only**: ClickHouse does not support re-org handling due to its architecture
- **Performance impact**: History tracking adds overhead to write operations
- **Storage growth**: History table grows with write volume until cleanup occurs

## Troubleshooting

**Missing primary key error**: Ensure all tables have primary keys defined and that substreams output matches the schema.

**Performance issues**: Consider adjusting flush intervals or using undo-buffer-size for high-throughput scenarios.

**History table growth**: Monitor cleanup operations and finality progression to ensure proper pruning.