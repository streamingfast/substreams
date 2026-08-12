# Proto Annotations Reference

When the output module of a SQL sink does not produce `DatabaseChanges`, the sink derives the database schema from the module's protobuf definition (relational mappings). This page is the reference for the annotations and type mappings driving that schema generation. For a guided walkthrough, see [Using Relational Mappings](../../how-to-guides/sinks/sql/relational-mappings.md).

Annotations come from `sf/substreams/sink/sql/schema/v1/schema.proto`:

```proto
import "sf/substreams/sink/sql/schema/v1/schema.proto";
```

## Table Options

Annotate a message with `(schema.table)` to control the table it maps to:

```proto
message MyTable {
  option (schema.table) = {
    name: "my_table"
    clickhouse_table_options: {
      order_by_fields: [{name: "id"}]
      partition_fields: [{name: "created_date", function: toYYYYMM}]
      index_fields: [{
        field_name: "status"
        name: "status_idx"
        type: bloom_filter
        granularity: 4
      }]
    }
  };
}
```

### Child Tables

Nested messages can map to child tables with a foreign key to their parent:

```proto
message OrderItem {
  option (schema.table) = {
    name: "order_items",
    child_of: "orders on order_id"
  };

  string item_id = 1;
  int64 quantity = 2;
}
```

## Field Options

Annotate individual fields with `(schema.field)`:

```proto
// Primary key
string id = 1 [(schema.field) = { primary_key: true }];

// Foreign key relationship
string user_id = 2 [(schema.field) = { foreign_key: "users on id"}];

// Unique constraint
string email = 3 [(schema.field) = { unique: true }];

// Custom column name
string user_address = 4 [(schema.field) = { name: "wallet_address" }];

// String-to-numeric conversion for large values
string token_amount = 5 [(schema.field) = { convertTo: { uint256{} } }];

// Decimal conversion with specific scale
string price = 6 [(schema.field) = { convertTo: { decimal128{ scale: 18 } } }];
```

## Type Mappings

Protobuf types map to SQL types as follows:

| Protobuf Type | PostgreSQL Type | ClickHouse Type |
|---------------|-----------------|-----------------|
| `string` | `VARCHAR(255)` | `String` |
| `int32`, `sint32`, `sfixed32` | `INTEGER` | `Int32` |
| `int64`, `sint64`, `sfixed64` | `BIGINT` | `Int64` |
| `uint32`, `fixed32` | `NUMERIC` | `UInt32` |
| `uint64`, `fixed64` | `NUMERIC` | `UInt64` |
| `float` | `DECIMAL` | `Float32` |
| `double` | `DOUBLE PRECISION` | `Float64` |
| `bool` | `BOOLEAN` | `Bool` |
| `bytes` | `TEXT` | `String` |
| `google.protobuf.Timestamp` | `TIMESTAMP` | `DateTime` |
| `repeated <type>` | `<type>[]` | `Array(<type>)` |

### Extended Numeric Types

String fields holding numeric values larger than native integer ranges can be converted with the `convertTo` field option:

| String Conversion Type | PostgreSQL Type | ClickHouse Type | Use Case |
|------------------------|-----------------|-----------------|----------|
| `Int128` | `NUMERIC(39,0)` | `Int128` | 128-bit signed integers |
| `UInt128` | `NUMERIC(39,0)` | `UInt128` | 128-bit unsigned integers |
| `Int256` | `NUMERIC(78,0)` | `Int256` | 256-bit signed integers |
| `UInt256` | `NUMERIC(78,0)` | `UInt256` | 256-bit unsigned integers |
| `Decimal128` | `NUMERIC(38,scale)` | `Decimal128(precision,scale)` | 128-bit decimals |
| `Decimal256` | `NUMERIC(76,scale)` | `Decimal256(precision,scale)` | 256-bit decimals |

Typical uses: token amounts exceeding the uint64 range, high-precision decimal arithmetic, 256-bit hashes or identifiers stored as strings.

## ClickHouse-Specific Options

ClickHouse tables require the `clickhouse_table_options` annotation on each table: at minimum `order_by_fields`, optionally `partition_fields` and `index_fields`.

```proto
message Transfer {
  option (schema.table) = {
    name: "transfers"
    clickhouse_table_options: {
      // Order by transaction hash and block for efficient range queries
      order_by_fields: [
        {name: "trx_hash"},
        {name: "_block_number_"},
        {name: "from"},
        {name: "to"}
      ]
      // Partition by month for efficient time-based queries
      partition_fields: [{name: "_block_timestamp_", function: toYYYYMM}]
      // Add indexes for specific query patterns
      index_fields: [{
        field_name: "from"
        name: "from_idx"
        type: bloom_filter
        granularity: 4
      }]
    }
  };
}
```

### Table Engine

Tables are created with the `ReplacingMergeTree` engine:

```sql
ENGINE = ReplacingMergeTree(_version_, _deleted_)
SETTINGS allow_experimental_replacing_merge_with_cleanup = 1
```

This provides automatic deduplication based on the `_version_` column and soft deletes through the `_deleted_` column for handling chain reorgs.

### Querying with Reorg Safety

Filter out deleted records for current-state queries:

```sql
SELECT * FROM transfers.transfers
WHERE _deleted_ = 0
```

For aggregations (including materialized views), use the additive pattern so reorg corrections cancel out:

```sql
SELECT
    sum(if(_deleted_, -amount, amount)) AS net_volume
FROM transfers.transfers
```

> **📚 Deep Dive**: See the [ClickHouse Showcase](https://github.com/streamingfast/substreams-sink-clickhouse-showcase) and its [Deep Dive](https://github.com/streamingfast/substreams-sink-clickhouse-showcase/blob/main/DEEP_DIVE.md) for a production-grade example with materialized views and partitioning strategies.

## Constraints and performance

The sink loads without database constraints and creates them afterwards, because the
difference is not marginal. Measured through binary COPY over 500,000 rows
(`TestConstraintCost` in `sink/sql/db_proto/benchmarks`):

| what the load runs with | duration | vs bare |
|---|--:|--:|
| no constraints | 137ms | 1.0x |
| primary keys | 554ms | 4.0x slower |
| primary keys + unique | 809ms | 5.9x slower |
| primary keys + unique + foreign keys | 3.798s | **27.7x slower** |
| bare load, then all of them created | 449ms | 3.3x slower |

Building the constraints after the load costs 8.5x less than loading with them in place,
for exactly the same schema. So the default is: load bare, then create them when you say
so.

```bash
# the load, creating the constraints itself once it reaches chain HEAD
substreams sink postgres substreams.yaml --dsn $DSN

# or: load bare, and create them yourself in a maintenance window
substreams sink postgres substreams.yaml --dsn $DSN --apply-constraints=manual
substreams sink postgres constraints apply substreams.yaml --dsn $DSN
```

`--apply-constraints` says when they are created:

| value | when |
|---|---|
| `auto` *(default)* | the sink creates them once the backfill reaches chain HEAD, or at the end of a bounded run |
| `manual` | left to `sink postgres constraints apply` |
| `always` | created before the load, which is the 27.7x row above |

`auto` is the default even though the pass is stop-the-world — every index built and every
foreign key validated, tables locked while it runs. A backfill that ends with no
constraints is a silent wrong result that looks like success; a stall is at least a
visible one. Use `manual` to put that pass in a maintenance window instead.

Running `constraints apply` again is safe — the constraints already in place are left
alone — so it also back-fills a schema an earlier run created without them.
`constraints drop` is the inverse, and the escape hatch after `--apply-constraints=always`
or before resuming a backfill.

Until it has run, the tables carry no primary keys, no unique constraints, no foreign keys
and therefore **no indexes at all**, so queries are sequential scans and duplicate ids are
not rejected. A reorg is still undone correctly: the rows of the undone blocks are deleted
from each table explicitly rather than through a cascade.

- `--apply-constraints`: when they are created — `manual` (default, the command above),
  `head` (the sink creates them itself once the backfill reaches chain HEAD), or `upfront`
  (before the load, paying the 27.7x).
- `--disable-foreign-keys`: never create foreign keys, including the one every table has to
  the block table.
- `--disable-primary-keys`: tables that go without a primary key, or `all`.
- `--disable-unique-constraints`: tables whose unique constraints are left out, or `all`.

## Which strategy for which database

The constraints are the same either way; what changes is when you pay for them and what
the database can do in the meantime. Pick by how long the backfill is.

### A development database, or anything that loads in a few minutes

Create the constraints before the load and forget about them. The database is correct from
the first block, and on a small dataset the 32x is a couple of seconds.

```bash
substreams sink postgres substreams.yaml --dsn $DSN \
  --apply-constraints=upfront \
  -e mainnet.eth.streamingfast.io:443 -s 20000000 -t +50000
```

One command, nothing to do afterwards. This is also the right choice for a chain that is
already near its head, where there is no backfill to speak of.

### A production backfill you can babysit — millions of rows

Load bare, and let the sink create the constraints when it catches up to the chain head.
You get the fast path for the whole backfill and a correct database at the end, at the
cost of one pause when it switches over.

```bash
substreams sink postgres substreams.yaml --dsn $DSN \
  --apply-constraints=head \
  -e mainnet.eth.streamingfast.io:443 -s 20000000
```

What happens, in order: rows are buffered on disk and loaded with binary COPY; the first
live block drains the buffer and switches to direct inserts; the constraints are then
created, locking each table while its indexes are built and its foreign keys validated;
the sink resumes following the chain. Expect that pause to last minutes on a table of tens
of millions of rows — the sink stops consuming the stream while it happens.

### A large or busy production database — tens of millions of rows and up

Keep the two apart, so the lock window is yours to schedule.

**Step 1 — create the schema:**

```bash
substreams sink postgres setup substreams.yaml --dsn $DSN
```

**Step 2 — backfill, bare** (this is the default, no flag needed):

```bash
substreams sink postgres substreams.yaml --dsn $DSN \
  -e mainnet.eth.streamingfast.io:443 -s 20000000
```

Let it run to the chain head. It logs, once the backfill is done, that the schema has no
constraints yet and names the command below. Queries against the tables work but are
sequential scans until step 3.

**Step 3 — create the constraints, in a maintenance window:**

```bash
substreams sink postgres constraints apply substreams.yaml --dsn $DSN
```

Every table is locked while its indexes are built and its foreign keys validated. Run it
when a stall is acceptable. It is idempotent, so a run that was interrupted can simply be
repeated, and it also back-fills a schema that was created without constraints long ago.

The sink can keep running during step 3 — it will be blocked on the locks and resume when
they are released — but on a large database it is calmer to stop it, apply, and restart.

### A database you query but never join — analytics, exports

Foreign keys are what a load pays most for, and referential integrity is what you are
least likely to need in an append-only table. Keep the primary keys, which is what gives
you an index on the entity id, and drop the rest:

```bash
# during the backfill: nothing to do, the default already loads bare

# afterwards:
substreams sink postgres constraints apply substreams.yaml --dsn $DSN \
  --disable-foreign-keys
```

Loading with primary keys and uniques in place costs 5.9x against 32x with foreign keys,
so if you would rather have them from the start:

```bash
substreams sink postgres substreams.yaml --dsn $DSN \
  --apply-constraints=upfront --disable-foreign-keys
```

### A bulk load into a throwaway database

No constraints at all, ever. The fastest the sink goes, and the least the database can
tell you.

```bash
substreams sink postgres substreams.yaml --dsn $DSN \
  --disable-foreign-keys --disable-primary-keys=all --disable-unique-constraints=all
```

Nothing is indexed, so add whatever index your queries actually need by hand afterwards
rather than paying for the ones the schema would have implied.

### Per-table exceptions

The switches take table names, so a single hot table can be treated differently from the
rest — here every table keeps its constraints except one whose primary key would be
expensive and is never queried by id:

```bash
substreams sink postgres constraints apply substreams.yaml --dsn $DSN \
  --disable-primary-keys=order_items
```

## Performance flags

A fast initial import is the default: the sink creates no constraints during the load,
spools rows to local disk and loads them with binary COPY from a background goroutine, so
the stream never waits on the database.

The flags are grouped by what they spend, and `substreams sink postgres --help` prints
them under those headings.

**`--decode-*` — CPU.** These size the decode stage and change nothing about what the
database sees.

| flag | default | what it does |
|---|---|---|
| `--decode-workers` | one per core less one, capped at 8 | blocks unmarshalled and walked concurrently |
| `--decode-batch-size` | 4 × workers | blocks held in memory and decoded together |

`--block-batch-size` is the old name of `--decode-batch-size` and still works for one
release. It used to size the database transaction; the spool took that job, and the two
are separate concerns now.

**`--db-write-*` — one commit to the database.** A commit is one spooled segment. The sink
measures each one and sizes the next toward the target, so the number of blocks or rows in
a commit is a result rather than a setting — block payloads differ by orders of magnitude
across chains, which is exactly what makes a fixed count give wildly variable durations.

| flag | default | what it does |
|---|---|---|
| `--db-write-target-duration` | `3s` | how long one commit should take |
| `--db-write-max-size` | `512MiB` | ceiling on the segment size the sizer may choose |

Lower the target for smaller, more frequent commits: gentler on a database shared with
something else, cursor advances more often, less re-streamed after a crash.

**`--spool-*` — how far ahead of the database the stream may run.** Both PostgreSQL and
ClickHouse spool.

| flag | default | what it does |
|---|---|---|
| `--spool-dir` | `./localdata/spool` | where pending segments are written |
| `--spool-max-size` | `8GiB` | disk budget; the stream is held once it is reached |
| `--spool-max-idle` | `10s` | commit the open segment when no new row has reached it for this long |

`--spool-max-size` is the only bound on the backlog. `--spool-max-idle` is what keeps a
stalled stream from sitting on rows indefinitely: without it the cursor would not advance
and those blocks would be streamed, and paid for, again on restart.

**`--write-mode` — how a segment reaches the database.**

| value | how |
|---|---|
| `copy` | `COPY ... FROM STDIN (FORMAT BINARY)` per table — measured at ~7x the multi-row INSERT path |
| `batch-insert` | one multi-row INSERT per table |
| `row-insert` | one prepared INSERT per row |
| `auto` *(default)* | `copy` on PostgreSQL, `batch-insert` on ClickHouse, `row-insert` for a schema whose foreign keys form a cycle |

A cycle has no table order, so grouping rows by table cannot keep a parent ahead of its
children, while the walk itself always does — hence `row-insert` as the fallback. An
explicit mode the driver or the schema cannot support is an error, not a downgrade.

The spool is a backfill tool, so the sink stops using it on its own once the stream reaches
the chain head: on the first live block everything spooled is applied and the inserts go
straight to the database from then on, where a block is queryable as soon as it arrives.
Every flag above stops applying at that point.

## Troubleshooting

1. **Proto import errors**: ensure all required proto files are in your import paths
2. **Schema annotation errors**: verify you import `sf/substreams/sink/sql/schema/v1/schema.proto`
3. **Module output type errors**: ensure your Substreams module outputs the expected proto message
4. **String conversion errors**: verify that string fields marked for numeric conversion contain valid numeric values
5. **ClickHouse partition errors**: ensure partition functions match your data types (e.g. `toYYYYMM` for timestamps)
