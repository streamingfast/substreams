# Delta Updates

Delta updates allow you to perform atomic database operations like increments, decrements, and conditional updates directly at the database level, without needing to read existing values first.

> **Note**: Delta updates are currently a **PostgreSQL-only feature**. ClickHouse support is not available at this time.

## Overview

Traditional database updates require a read-modify-write pattern: fetch the current value, compute the new value, then write it back. This approach has two drawbacks:

1. **Performance overhead** - Reading existing values for every update is expensive
2. **Concurrency issues** - Race conditions can occur between the read and write

Delta updates solve both problems by expressing the update intent declaratively. The database handles the actual computation atomically.

## Requirements

Delta update operations require:

- `substreams-sink-database-changes` Rust crate version **>= 4.0.0**
- The SQL sink included in the `substreams` CLI (equivalent to `substreams-sink-sql` **>= 4.12.0**)
- PostgreSQL as the target database

## Supported Operations

The following delta operations are available on `create_row()`, `update_row()`, and `upsert_row()`:

| Operation | SQL Equivalent | Description |
|-----------|----------------|-------------|
| `add()` | `column = COALESCE(column, 0) + value` | Atomically increment a numeric column |
| `sub()` | `column = COALESCE(column, 0) - value` | Atomically decrement a numeric column |
| `max()` | `column = GREATEST(column, value)` | Retain the maximum value seen |
| `min()` | `column = LEAST(column, value)` | Retain the minimum value seen |
| `set_if_null()` | `column = COALESCE(column, value)` | Set only if the column is currently NULL |

All operations handle NULL values safely using `COALESCE`, so you don't need to worry about NULL arithmetic.

## Usage

Delta operations are chained on the `upsert_row()` function from the `DatabaseChanges` API. You can mix regular `set()` calls with delta operations:

```rust
use substreams_database_change::pb::sf::substreams::sink::database::v1::DatabaseChanges;
use substreams_database_change::tables::Tables;

fn db_out(data: MyData) -> Result<DatabaseChanges, Error> {
    let mut tables = Tables::new();

    tables.upsert_row("accounts", &data.account_id)
        .set("owner", &data.owner)           // Regular set
        .add("balance", data.deposit)         // Increment balance
        .sub("debt", data.payment)            // Decrement debt
        .max("high_score", data.score)        // Track maximum
        .min("best_time", data.duration)      // Track minimum
        .set_if_null("created_at", data.timestamp);  // Set once

    Ok(tables.to_database_changes())
}
```

## Example: OHLCV Candlestick Data

A practical use case for delta updates is building candlestick (OHLCV) charts from trading data. Each candle needs to track:

- **Open**: First price in the window (set once, never update)
- **High**: Maximum price seen
- **Low**: Minimum price seen
- **Close**: Latest price (always update)
- **Volume**: Cumulative trading volume

```rust
tables.upsert_row("candles", [
    ("pool_id", pool_id),
    ("interval", interval.to_string()),
    ("timestamp", window_start.to_string()),
])
.set_if_null("open", &price)    // First price wins
.set("close", &price)           // Always update to latest
.max("high", tick)              // Track highest
.min("low", tick)               // Track lowest
.add("volume", volume)          // Accumulate
.add("trade_count", 1i64);      // Count trades
```

This pattern processes each swap event independently. The database handles all the aggregation logic atomically.

For a complete working example, see the [Uniswap V4 Candles Demo](https://github.com/streamingfast/substreams-eth-uni-v4-demo-candles).

## Migration Notes

If you're upgrading from an earlier version of `substreams-sink-database-changes`, note that the crate package path was reorganized in version 4.0.0. Update your imports:

```rust
// Old path (deprecated)
use substreams_database_change::pb::database::DatabaseChanges;

// New path
use substreams_database_change::pb::sf::substreams::sink::database::v1::DatabaseChanges;
```

## Further Reading

- [substreams-sink-database-changes Crate Documentation](https://docs.rs/substreams-database-change/latest/substreams_database_change/) - API reference for delta update methods
- [substreams-sink-sql Repository](https://github.com/streamingfast/substreams-sink-sql) - Sink SQL source code and documentation
- [Uniswap V4 Candles Demo](https://github.com/streamingfast/substreams-eth-uni-v4-demo-candles) - Complete working example using delta updates
