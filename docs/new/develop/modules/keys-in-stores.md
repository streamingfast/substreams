---
description: Using keys in stores
---

# Keys in stores

We use store modules to aggregate the data in the underlying key-value storage. It is important to have a system for organizing your keys to be able to efficiently retrieve, filter and free them when needed.

In most cases, you will encode data into your keys into segmented parts, adding a prefix as namespace for example `user` and `<address>` joined together using a separator.  Segments in a key are conventionally joined with `:` as a separator.

Here are some examples,
- `Pool:{pool_address}:volumeUSD` - `{pool_address}` pool total traded USD volume
- `Token:{token_addr}:volume` - total `{token_addr}` token volume traded
- `UniswapDayData:{day_id}:volumeUSD` - `{day_id}` daily USD trade volume
- `PoolDayData:{day_id}:{pool_address}:{token_addr}:volumeToken1` - total `{day_id}` daily volume of `{token_addr}` token that went through a `{pool_address}` pool in token1 equivalent

In the example of a counter store below, we increment transaction counters for different metrics that we could use in the downstream modules:
```rust
#[substreams::handlers::store]
pub fn store_total_tx_counts(clock: Clock, events: Events, output: StoreAddBigInt) {
    let timestamp_seconds = clock.timestamp.unwrap().seconds;
    let day_id = timestamp_seconds / 86400;
    let hour_id = timestamp_seconds / 3600;
    let prev_day_id = day_id - 1;
    let prev_hour_id = hour_id - 1;

    for event in events.pool_events {
        let pool_address = &event.pool_address;
        let token0_addr = &event.token0;
        let token1_addr = &event.token1;

        output.add_many(
            event.log_ordinal,
            &vec![
                format!("pool:{pool_address}"),
                format!("token:{token0_addr}"),
                format!("token:{token1_addr}"),
                format!("UniswapDayData:{day_id}"),
                format!("PoolDayData:{day_id}:{pool_address}"),
                format!("PoolHourData:{hour_id}:{pool_address}"),
                format!("TokenDayData:{day_id}:{token0_addr}"),
                format!("TokenDayData:{day_id}:{token1_addr}"),
                format!("TokenHourData:{hour_id}:{token0_addr}"),
                format!("TokenHourData:{hour_id}:{token1_addr}"),
            ],
            &BigInt::from(1 as i32),
        );
    }
}
```

In the downstream modules consuming this store, you can query the store by key in `get` mode. Or, an even more powerful approach would be to filter needed store deltas by segments. `key` module of the `substreams` crates offers several helper functions. Using these functions you can extract the first/last/nth segment from a key:

```rust
for delta in deltas.into_iter() {
    let kind = key::first_segment(delta.get_key());
    let address = key::segment_at(delta.get_key(), 1);
    // Do something for this kind and address
}
```

`key` module also provides corresponding `try_` methods that don't panic:
- `first_segment` & `try_first_segment`
- `last_segment` & `try_last_segment`
- `segment_at` & `try_segment_at`

For a full example see [Uniswap V3 Substreams](https://github.com/streamingfast/substreams-uniswap-v3/blob/ca90fe3908a76905b43e05f0522e1e9338d88972/src/lib.rs#L1139-L1163)

## Links
* [Uniswap-v3 Subgraph and Substreams](https://github.com/streamingfast/substreams-uniswap-v3)
* [Key module documentation](https://docs.rs/substreams/latest/substreams/key/index.html)
