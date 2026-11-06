# ClickHouse Stateful Mode

This applies to the ClickHouse sink in Database Changes mode (`substreams sink clickhouse` with a
module outputting `DatabaseChanges`).

By default the ClickHouse sink only inserts rows: upserts, updates and deletes are refused, field
update operations (`ADD`, `MAX`, `MIN`, `SET_IF_NULL`, see [Delta Updates](delta-updates.md)) are not
applied, and reorgs need the undo buffer. With `--clickhouse-stateful`, the sink computes each row's
new state itself and supports every operation, reorgs included. A module gives the same rows on
ClickHouse as on PostgreSQL: the test suite checks every operation type against both.

## Turning it on

Pass `--clickhouse-stateful` to both `setup` (it creates the `substreams_journal` and history tables)
and `run`:

```bash
substreams sink clickhouse setup "$DSN" my-substreams.spkg --clickhouse-stateful
substreams sink clickhouse run "$DSN" my-substreams.spkg --clickhouse-stateful
```

With `--undo-buffer-size=0` (the default), blocks are written as soon as they arrive and reverted
in the database on a reorg. With an undo buffer, ClickHouse only receives
blocks once confirmed by that many blocks.

## Tables

The sink sorts the tables your module writes in two kinds at startup, and stops with a message
saying what to add to a table that fits neither. Other tables in the database (the targets of your
materialized views, say) are left alone.

**Keyed tables** take every operation. They use `ReplacingMergeTree(_version_, _deleted_)`, are
ordered by their primary key, and have the two columns the sink manages:

```sql
CREATE TABLE balances (
    id String,
    amount UInt256,
    tx_count UInt64,
    _version_ UInt64,
    _deleted_ UInt8
) ENGINE = ReplacingMergeTree(_version_, _deleted_)
ORDER BY id
SETTINGS index_granularity = 256;
```

**Append tables** take inserts only. A plain `MergeTree` table with a `_version_ UInt64` column, and
a `_block_num_ UInt64` column when reorgs are handled in the database:

```sql
CREATE TABLE transfers (
    id String,
    amount UInt256,
    _version_ UInt64,
    _block_num_ UInt64
) ENGINE = MergeTree ORDER BY id;
```

or, for tables read by materialized views, a `CollapsingMergeTree(_sign_)` with an `_sign_ Int8`
column too (see [Materialized views](#materialized-views)). Its sorting key must identify a row
(a transaction hash and log index, say), as `CollapsingMergeTree` merges rows with the same sorting
key:

```sql
CREATE TABLE trades (
    id String,
    amount Int64,
    _version_ UInt64,
    _block_num_ UInt64,
    _sign_ Int8
) ENGINE = CollapsingMergeTree(_sign_) ORDER BY id;
```

Your module never sets `_version_`, `_deleted_`, `_block_num_` or `_sign_`: the sink does. Other
MergeTree variants, like `SummingMergeTree`, merge rows together and are refused.

Before each flush, the sink reads the current rows of the keys the flush touches, one query per
keyed table. ClickHouse finds a key by reading the whole granule it sits in, so keep keyed tables'
`index_granularity` small: 256 instead of the default 8192 made these reads 3 to 4 times faster on a
100 million row table.

## How operations apply

- `INSERT` creates a row and fails if the key exists; `UPSERT` creates or updates it; `UPDATE` and
  `DELETE` of a missing key do nothing.
- `ADD`, `MAX` and `MIN` count NULL as 0, and `SET_IF_NULL` keeps a value that is not NULL. An
  upsert's value is converted to the column's type first (`-0.5` is `-1` in an integer column).
- Results are exact: decimals keep the column's scale, and a fractional result in an integer column
  is rounded half away from zero.

## Reading the data

A changed row is written again in full with a higher `_version_`, and a deleted row as a
`_deleted_ = 1` row. ClickHouse drops the older versions when it merges parts, in the background, so
read keyed tables with `FINAL` to get exactly one current row per key:

```sql
SELECT id, amount FROM balances FINAL WHERE _deleted_ = 0;
```

In a `CollapsingMergeTree(_sign_)` table, a removed row is cancelled by a copy with `_sign_ = -1`.
Count with the sign, or read with `FINAL`:

```sql
SELECT id, sum(amount * _sign_) FROM trades GROUP BY id HAVING sum(_sign_) > 0;
```

## Reorgs

With reorgs handled in the database, each flush records in the history table, for each keyed row a
reversible block changes, the row as it was before that block. On an undo, the sink writes back each
row as it was before the first reverted block, and removes the append rows of the reverted blocks:
deleted from a `MergeTree`, cancelled by `-1` rows in a `CollapsingMergeTree(_sign_)`. History of
final blocks is pruned as the chain moves on.

## Materialized views

Materialized views run on each insert: they see every row written, and never see a delete.

- **Append tables:** use `CollapsingMergeTree(_sign_)` for tables read by materialized views, and
  write the views with the sign (`sum(amount * _sign_)`, `sum(_sign_)` for counts). Rows removed by
  a reorg, or by the rollback of a flush cut short, are cancelled by `-1` rows the views see, so
  their results stay right. On a plain `MergeTree`, removed rows stay counted in the views.
- **Keyed tables:** each change writes the row again, so an aggregating view would count each
  version. Use a view, a refreshable materialized view (`REFRESH EVERY ...`) reading with `FINAL`, or
  a materialized view that only reshapes rows into another `ReplacingMergeTree` ordered by the same
  key with the same `_version_`.

The sink logs a warning at startup for each table it writes that a materialized view reads.

A view stays right as long as ClickHouse delivers each insert to it. If an insert reaches a table
but fails in one of its views (an error in the view, a server crash mid-insert), ClickHouse keeps
the table's rows, and the sink, which only sees the table, cannot tell the view missed them: rebuild
the view's target from its source table after such a failure.

## Crash safety

ClickHouse has no transaction across tables, so a flush cut short (a crash, a failed write) can
leave some tables written without the cursor. Before writing, a flush records in
`substreams_journal` the rows it is about to change as they were, and the cursor it starts from. If
its cursor is not written, the flush is rolled back before anything else: its rows are written
again as they were, with a still higher `_version_`, and its append rows removed. The stream then
resumes from the stored cursor, so no change is applied twice. This runs on the next attempt when
the sink retries a flush, and on startup after a crash (once the sink holds the writer lock, with
`--standby-lock`). A revert records the block and cursor it reverts to: one cut short is completed,
cursor included, before anything else, whatever the stream sends next.

A write the sink gave up on (a timeout, a lost connection) may still be running in ClickHouse.
Before a rollback, and on startup, the sink kills its writes still running (each has a query ID
starting with `substreams-sink:<database>:`) and waits for them to stop, so none lands after the
rollback. A standby taking over does the same with the previous holder's writes.

Inserts are made synchronous (`async_insert = 0`), whatever the DSN or server settings, as the
journal, the rows and the cursor must be written in that order.

## One writer, and standby sinks

Each flush and revert first checks that the stored cursor is still the one the sink last read or
wrote, and the sink stops with `the stored cursor is not the one this sink last read or wrote`
otherwise (see [Standby Sinks](standby-sinks.md)).

`--standby-lock` works too. The lock is a lease in a `KeeperMap` table, which needs ClickHouse Keeper
(or ZooKeeper) and `keeper_map_path_prefix` in the server configuration; ClickHouse Cloud has both.
The active sink renews its lease every `--standby-lock-poll-interval`; a standby takes over once the
lease expires (3 poll intervals, at least 15 seconds), after the active sink stopped or lost the
database. Lease times are the ClickHouse server's, so the machines' clocks don't matter, and leases
are taken with `keeper_map_strict_mode`, so two sinks never hold one at once.

ClickHouse cannot make the other tables' writes conditional on the lease, so the active sink checks
its lease, with a margin of a third of it, before each write (the journal, each table, the cursor)
and again just before a batch of rows is sent.
To cover writes the previous holder may still have in flight, a standby taking over an expired
lease waits one more lease period before writing. A sink frozen between a check and its write for
longer than that (a paused VM, say) could still write after losing its lease: keep the poll interval
well above such pauses.

## Limits

- On a cluster, write to and read from the same replica, or use `insert_quorum` with
  `select_sequential_consistency`: a replica behind the others would give stale current rows.
- A query running during a flush can see some tables updated and others not yet.
- An operation changing a primary key column's value is refused.
- A new row takes a column's `DEFAULT` when it does not set it only if that default is a constant
  (`0`, `'none'`, `false`); otherwise NULL, or the type's zero value when the column is not
  `Nullable`. So `SET_IF_NULL` fills a column only if it is `Nullable`: a stored `0` or `''` is not
  NULL.
- A keyed table's columns must be types the sink can read back and write again, as each operation
  writes the whole row: `Map`, `IPv4` and `IPv6` columns, for instance, are refused at startup.
