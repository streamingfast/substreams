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

## Performance Flags

A fast initial import is the default: the sink creates no constraints, inserts rows in
batches and buffers them on local disk before loading them with binary COPY.

```bash
substreams sink postgres substreams.yaml --dsn $DSN \
  --block-batch-size 100
```

- `--block-batch-size`: number of blocks to process at a time (default: 25)
- `--local-buffer`: directory the rows are buffered in (default: `./localdata/local-buffer`);
  an empty value turns the buffer off
- `--local-buffer-max-size`: disk budget for that buffer (default: `8GiB`). The stream is
  held once the buffer fills, so the database is never outrun by more than this.

Once the sink is close to chain HEAD, constraints can be turned on:

```bash
substreams sink postgres substreams.yaml --dsn $DSN --with-constraints
```

- `--with-constraints`: add the primary keys, unique and foreign key constraints, creating
  the ones an earlier run left out.

  This prevents the performance enhancements above — multi-row inserts and the local
  buffer are both disabled while constraints are on — and will slow a big initial sync of
  the database down considerably, so we recommend that it stay off until you are close to
  chain HEAD. Enabling the SQL constraints on a populated database can also take a long
  time, locking the tables while it runs. Passing `--local-buffer` together with it is an
  error rather than a silent downgrade.

## Troubleshooting

1. **Proto import errors**: ensure all required proto files are in your import paths
2. **Schema annotation errors**: verify you import `sf/substreams/sink/sql/schema/v1/schema.proto`
3. **Module output type errors**: ensure your Substreams module outputs the expected proto message
4. **String conversion errors**: verify that string fields marked for numeric conversion contain valid numeric values
5. **ClickHouse partition errors**: ensure partition functions match your data types (e.g. `toYYYYMM` for timestamps)
