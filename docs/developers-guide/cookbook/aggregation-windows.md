---
description: Building and freeing up aggregation windows
---

# Building and freeing up aggregation windows

Store module key-value storage can hold at most 1 GiB. It is usually enough if used correctly, but it is still a good idea (and sometimes even necessary) to free up unused keys. It is especially true for cases where you work with aggregation windows.

Consider this store module that aggregates hourly trade counter for each token:
```rust
#[substreams::handlers::store]
pub fn store_total_tx_counts(clock: Clock, events: Events, output: StoreAddBigInt) {
    let timestamp_seconds = clock.timestamp.unwrap().seconds;
    let hour_id = timestamp_seconds / 3600;
    let prev_hour_id = hour_id - 1;

    output.delete_prefix(0, &format!("TokenHourData:{prev_hour_id}:"));

    for event in events.pool_events {
        output.add_many(
            event.log_ordinal,
            &vec![
                format!("TokenHourData:{}:{}", hour_id, event.token0),
                format!("TokenHourData:{}:{}", hour_id, event.token1),
            ],
            &BigInt::from(1 as i32),
        );
    }
}
```

Let's break it down.

First, we use `Clock` input source to get the current and previous hour id for the block.

```rust
let hour_id = timestamp_seconds / 3600;
let prev_hour_id = hour_id - 1;
```

Then we build hourly keys for our counters and use `add_many` method to increment them. These counters will be consumed downstream by other modules.

```rust
output.add_many(
    event.log_ordinal,
    &vec![
        format!("TokenHourData:{}:{}", hour_id, event.token0),
        format!("TokenHourData:{}:{}", hour_id, event.token1),
    ],
    &BigInt::from(1 as i32),
);
```

Here's the trick. Since we don't need these counters outside of the hourly window, we can safely delete these key-value pairs for the previous hourly window and free up the memory.

This is done using `delete_prefix` method:
```rust
output.delete_prefix(0, &format!("TokenHourData:{prev_hour_id}:"));
```

## Links
* [Uniswap-v3 Subgraph and Substreams](https://github.com/streamingfast/substreams-uniswap-v3)