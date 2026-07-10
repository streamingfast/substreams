# FROM_PROTO Command Guide

The `from-proto` command allows you to run a Substreams SQL sink directly from a protocol buffer definition without needing to set up separate schema files. This command automatically generates the SQL schema from your protobuf definitions and runs the sink in a single step.

> **💡 See it in action**: Check out the [ClickHouse Showcase](https://github.com/streamingfast/substreams-sink-clickhouse-showcase) for a complete working example with USDC transfers, including advanced features like materialized views and partitioning strategies.

## Overview

The `from-proto` command streamlines the process of running a SQL sink by:
1. Reading your Substreams manifest and protobuf definitions
2. Automatically generating the SQL schema from proto annotations
3. Creating database tables based on the proto message structure
4. Running the sink to stream data from Substreams to your database

## Command Syntax

```bash
substreams sink from-proto <manifest> [output-module] --dsn <dsn>
```

### Arguments

- `<manifest>`: Path to your Substreams manifest file (substreams.yaml)
- `[output-module]`: Optional. Name of the output module to stream from (defaults to auto-detection)

### Common Flags

- `--dsn`: Database connection string (falls back to the `SUBSTREAMS_SINK_DSN` environment variable) - see [DSN Format](#dsn-format) section below
- `-e, --endpoint`: Substreams gRPC endpoint
- `-s, --start-block`: Start block number to stream from
- `-t, --stop-block`: Stop block to end stream at (default: 0, meaning no limit)
- `--no-constraints`: Skip adding database constraints (useful for fast initial imports)
- `--block-batch-size`: Number of blocks to process at a time (default: 25)

## DSN Format

The Data Source Name (DSN) specifies how to connect to your database. The format varies by database type:

### PostgreSQL DSN Format

```
postgres://[username[:password]@]host[:port]/database[?param1=value1&param2=value2]
```

**Examples:**
```bash
# Basic connection
postgres://localhost:5432/postgres

# With authentication (replace with actual credentials)
postgres://[username]:[password]@localhost:5432/mydb

# With SSL disabled and custom schema
postgres://localhost:5432/postgres?sslmode=disable&schemaName=orders

# Production example with SSL (replace placeholders)
postgres://[username]:[password]@[hostname]:5432/analytics?sslmode=require
```

**Common PostgreSQL Parameters:**
- `sslmode`: SSL connection mode (`disable`, `require`, `verify-ca`, `verify-full`)
- `schemaName`: Target schema name (defaults to `public`)
- `connect_timeout`: Connection timeout in seconds
- `application_name`: Application name for connection tracking

### ClickHouse DSN Format

```
clickhouse://[username[:password]@]host[:port]/database[?param1=value1&param2=value2]
```

**Examples:**
```bash
# Basic connection (default user, no password)
clickhouse://127.0.0.1:9000/order?secure=false

# With authentication (replace with actual credentials)
clickhouse://[username]:[password]@localhost:9000/analytics

# Secure connection (replace placeholders)
clickhouse://[username]:[password]@[hostname]:9440/mydb?secure=true

# With custom settings
clickhouse://localhost:9000/analytics?secure=false&compress=true&debug=true
```

**Common ClickHouse Parameters:**
- `secure`: Use TLS encryption (`true` or `false`)
- `compress`: Enable compression (`true` or `false`)
- `debug`: Enable debug logging (`true` or `false`)
- `dial_timeout`: Connection timeout
- `max_execution_time`: Query execution timeout

### Environment Variable Usage

For security, avoid hardcoding credentials in commands. Use environment variables:

```bash
# Set DSN as environment variable (replace with actual credentials)
export SUBSTREAMS_SINK_DSN="postgres://[username]:[password]@localhost:5432/mydb?sslmode=disable"

# Use in command
substreams sink from-proto substreams.yaml
```

### Database Setup Examples

**PostgreSQL with Docker:**
```bash
docker run --name postgres \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=analytics \
  -p 5432:5432 -d postgres:13

export SUBSTREAMS_SINK_DSN="postgres://postgres:password@localhost:5432/analytics?sslmode=disable"
```

**ClickHouse with Docker:**
```bash
docker run --name clickhouse \
  -p 9000:9000 -p 8123:8123 \
  -d clickhouse/clickhouse-server

export SUBSTREAMS_SINK_DSN="clickhouse://127.0.0.1:9000/default?secure=false"
```

## Substreams to Database Step-by-Step Workflow

### Step 1: Initialize Your Substreams Project

Start by creating a new Substreams project:

```bash
# Create a new Substreams project
substreams init

# Follow the interactive prompts to configure your project
# This will create the basic project structure with:
# - substreams.yaml (manifest file)
# - proto/ directory for protocol buffer definitions
# - src/ directory for your Rust code
# - Cargo.toml for Rust dependencies
```

### Step 2: Create Your Protocol Buffer Definition

Create a `.proto` file that defines your data structure with SQL schema annotations. Here's an example based on the [USDC transfers showcase](https://github.com/streamingfast/substreams-sink-clickhouse-showcase):

```proto
syntax = "proto3";
import "google/protobuf/timestamp.proto";
import "sf/substreams/sink/sql/schema/v1/schema.proto";

package transfers;

message Output {
  repeated Transfer transfers = 1;
}

message Transfer {
  option (schema.table) = {
    name: "transfers"
    clickhouse_table_options: {
      order_by_fields: [{name: "tx_hash"}, {name: "log_index"}]
      partition_fields: [{name: "_block_timestamp_", function: toYYYYMM}]
      index_fields: [{
        field_name: "from_addr"
        name: "from_idx"
        type: bloom_filter
        granularity: 4
      }, {
        field_name: "to_addr"
        name: "to_idx"
        type: bloom_filter
        granularity: 4
      }]
    }
  };

  string tx_hash = 1;
  uint32 log_index = 2;
  string from_addr = 3;
  string to_addr = 4;
  string amount = 5 [(schema.field) = { convertTo: { uint256{} } }]; // Large token amounts as string
  string contract_addr = 6;
  google.protobuf.Timestamp timestamp = 7;
}
```

### Step 3: Create Your Substreams Manifest

Create a `substreams.yaml` file that references your protobuf:

```yaml
specVersion: v0.1.0
package:
  name: 'usdc-transfers'
  version: v0.1.0
  doc: |
    USDC transfers extraction for SQL sink

protobuf:
  files:
    - transfers.proto
  importPaths:
    - ./proto

binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/usdc_transfers.wasm

modules:
  - name: map_transfer
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:transfers.Output

network: mainnet

sink:
  module: map_transfer
  type: sf.substreams.sink.sql.v1.Service
  config: {}
```

### Step 4: Implement Your Substreams Module

Create your Rust module that outputs data matching your proto definition:

```rust
use substreams::prelude::*;
use substreams_ethereum::pb::eth;

#[substreams::handlers::map]
fn map_transfer(block: eth::v2::Block) -> Result<transfers::Output, substreams::errors::Error> {
    let mut output = transfers::Output::default();
    
    // Process block data and extract USDC transfers
    for transaction in block.transaction_traces {
        for log in transaction.receipt.unwrap().logs {
            // Check if this is a USDC transfer event
            if is_usdc_transfer(&log) {
                let transfer = extract_transfer_data(&log, &transaction);
                output.transfers.push(transfer);
            }
        }
    }
    
    Ok(output)
}

// Helper functions would be implemented here
fn is_usdc_transfer(log: &eth::v2::Log) -> bool {
    // Implementation to check if log is USDC transfer
    // ...
}

fn extract_transfer_data(log: &eth::v2::Log, tx: &eth::v2::TransactionTrace) -> transfers::Transfer {
    // Implementation to extract transfer data from log
    // ...
}
```

### Step 4: Build Your Substreams

Build your Substreams with:

```bash
substreams build
```

### Step 5: Set Up Your Database

Start your database (PostgreSQL or ClickHouse):

**PostgreSQL:**
```bash
docker run --name postgres -e POSTGRES_PASSWORD=password -p 5432:5432 -d postgres:13
```

**ClickHouse:**
```bash
docker run --name clickhouse -p 9000:9000 -d clickhouse/clickhouse-server
```

### Step 6: Run the from-proto Command

Execute the `from-proto` command to automatically generate schema and start streaming:

**PostgreSQL:**
```bash
export SUBSTREAMS_SINK_DSN="postgres://postgres:password@localhost:5432/postgres?sslmode=disable"
substreams sink from-proto substreams.yaml
```

**ClickHouse:**
```bash
export SUBSTREAMS_SINK_DSN="clickhouse://127.0.0.1:9000/default?secure=false"
substreams sink from-proto substreams.yaml
```

## Proto Schema Annotations

The `from-proto` command relies on special protobuf annotations to generate the SQL schema. Here are the key annotations:

### Table Options

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

### Field Options

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

### Child Tables

For nested messages, you can create child tables:

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

## Supported Data Types

The `from-proto` command automatically maps protobuf types to SQL types:

### Basic Types

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

### Extended Numeric Types (v4.8.0+)

For handling large numeric values that exceed standard integer ranges, you can use string-to-numeric conversion:

| String Conversion Type | PostgreSQL Type | ClickHouse Type | Use Case |
|------------------------|-----------------|-----------------|----------|
| `Int128` | `NUMERIC(39,0)` | `Int128` | 128-bit signed integers |
| `UInt128` | `NUMERIC(39,0)` | `UInt128` | 128-bit unsigned integers |
| `Int256` | `NUMERIC(78,0)` | `Int256` | 256-bit signed integers |
| `UInt256` | `NUMERIC(78,0)` | `UInt256` | 256-bit unsigned integers |
| `Decimal128` | `NUMERIC(38,scale)` | `Decimal128(precision,scale)` | 128-bit decimals |
| `Decimal256` | `NUMERIC(76,scale)` | `Decimal256(precision,scale)` | 256-bit decimals |

#### Using String-to-Numeric Conversion

When your protobuf contains string fields that represent large numeric values, you can specify conversion:

```proto
message Transfer {
  // Convert string amount to UInt256 for precise arithmetic
  string amount = 1 [(schema.field) = { convertTo: { uint256{} } }];
  
  // Convert string balance to Decimal128 with 18 decimal places
  string balance = 2 [(schema.field) = { convertTo: { decimal128{ scale: 18 } } }];
}
```

This is particularly useful for:
- **Cryptocurrency amounts**: Token values that exceed uint64 range
- **Financial calculations**: High-precision decimal arithmetic
- **Large identifiers**: 256-bit hashes or IDs stored as strings

## Advanced Usage

### Custom Block Range

Stream specific block ranges:

```bash
substreams sink from-proto substreams.yaml \
  --start-block 1000000 \
  --stop-block 1001000
```

### Performance Optimization

For high-throughput scenarios:

```bash
substreams sink from-proto substreams.yaml \
  --no-constraints \
  --block-batch-size 100
```

### ClickHouse-Specific Options

For ClickHouse with additional configuration:

```bash
substreams sink from-proto substreams.yaml \
  --clickhouse-sink-info-folder ./clickhouse-info \
  --clickhouse-cursor-file-path ./cursor.txt
```

## ClickHouse Advanced Features

### ReplacingMergeTree Engine (v4.9.0+)

The sink automatically uses ClickHouse's `ReplacingMergeTree` engine with enhanced cleanup capabilities:

```sql
ENGINE = ReplacingMergeTree(_version_, _deleted_)
SETTINGS allow_experimental_replacing_merge_with_cleanup = 1
```

This provides:
- **Automatic deduplication** based on the `_version_` column
- **Soft deletes** using the `_deleted_` column for handling chain reorgs
- **Experimental cleanup** for better storage efficiency

### Partitioning and Ordering Strategies

Configure optimal data organization for query performance:

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

### Materialized Views for Aggregations

Create efficient pre-aggregated views that handle chain reorgs correctly:

```sql
CREATE MATERIALIZED VIEW transfers.monthly_transfers
    ENGINE = SummingMergeTree()
    PARTITION BY month
    ORDER BY month
AS
SELECT
    toYYYYMM(_block_timestamp_) AS month,
    sum(if(_deleted_, -toInt256(amount), toInt256(amount))) AS volume,
    sum(if(_deleted_, -1, 1)) AS transfer_count
FROM transfers.transfers
GROUP BY month;
```

The key pattern is using `if(_deleted_, -value, value)` to handle reorg corrections automatically.

### Querying Best Practices

For current state queries, filter out deleted records:
```sql
SELECT * FROM transfers.transfers 
WHERE _deleted_ = 0
```

For aggregations with reorg safety, use the additive pattern:
```sql
SELECT 
    sum(if(_deleted_, -amount, amount)) AS net_volume
FROM transfers.transfers
```

> **📚 Deep Dive**: See the [ClickHouse Showcase Deep Dive](https://github.com/streamingfast/substreams-sink-clickhouse-showcase/blob/main/DEEP_DIVE.md) for detailed explanations of protobuf-to-schema mapping and materialized view patterns.

## Troubleshooting

### Common Issues

1. **Proto import errors**: Ensure all required proto files are in your import paths
2. **Schema annotation errors**: Verify you're importing `sf/substreams/sink/sql/schema/v1/schema.proto`
3. **Database connection issues**: Check your DSN format and database accessibility (see [DSN Format](#dsn-format) section)
4. **Module output type errors**: Ensure your Substreams module outputs the expected proto message
5. **String conversion errors**: Verify that string fields marked for numeric conversion contain valid numeric values
6. **ClickHouse partition errors**: Ensure partition functions match your data types (e.g., `toYYYYMM` for timestamps)

### Debug Tips

1. Use `substreams info` to verify your manifest structure
2. Check database logs for schema creation issues  
3. Verify your protobuf definitions compile correctly
4. Test with a small block range first (`--start-block` and `--stop-block`)

## Examples

### Complete Working Examples

1. **[ClickHouse Showcase](https://github.com/streamingfast/substreams-sink-clickhouse-showcase)** - Production-ready USDC transfers example featuring:
   - Real-world protobuf definitions with ClickHouse optimizations
   - String-to-UInt256 conversion for token amounts
   - Monthly partitioning and efficient ordering
   - Materialized views for aggregations
   - Docker setup for easy testing

2. **[Test Project](db_proto/test/substreams/order/)** - Comprehensive test suite demonstrating:
   - Complex proto definitions with relationships
   - Various data types and constraints
   - Child table relationships
   - PostgreSQL and ClickHouse compatibility


## Version Compatibility and Recent Changes

### v4.9.0 (Latest)
- **ClickHouse Improvements**: Removed deprecated `ClickhouseReplacingField` functionality
- **Enhanced ReplacingMergeTree**: Streamlined logic with `allow_experimental_replacing_merge_with_cleanup` setting
- **Better Performance**: Optimized table engine configuration for production workloads

### v4.8.0
- **Extended Numeric Types**: Added support for `Int128`, `UInt128`, `Int256`, `UInt256`, `Decimal128`, and `Decimal256`
- **String-to-Numeric Conversion**: Convert string fields to native numeric types for better performance
- **Enhanced Type Mapping**: Improved PostgreSQL and ClickHouse type mapping system

### Migration Notes
- **From v4.7.x to v4.8.0+**: No breaking changes, new numeric types are opt-in via field annotations
- **From v4.8.x to v4.9.0**: ClickHouse users may see improved performance due to ReplacingMergeTree optimizations
- **Existing deployments**: Continue to work without changes, new features available for new schema definitions
