---
description: Dynamic data sources and Substreams
---

# Dynamic data sources and Substreams

Using Factory contract is a quite common pattern used by dApps when the main smart contract deploys and manages multiple identical associated contracts, i.e. one smart contract for each Uniswap or Curve swap pool.

When developing traditional subgraphs, you could use [data source templates](https://thegraph.com/docs/en/developing/creating-a-subgraph/#data-source-templates) approach to keep track of such dynamically deployed smart contracts.

Here's how you can achieve that with Substreams.

We'll be using Uniswap V3 example where the Factory creates and deploys its smart contract for each pool.

You start with a simple map module that emits all pool creation events:
```yaml
- name: map_pools_created
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:uniswap.types.v1.Pools
```

```rust
#[substreams::handlers::map]
pub fn map_pools_created(block: Block) -> Result<Pools, Error> {
    Ok(Pools {
        pools: block
            .events::<abi::factory::events::PoolCreated>(&[&UNISWAP_V3_FACTORY])
            .filter_map(|(event, log)| {
                // skipped: extracting pool information from the transaction
                Some(Pool {
                    address,
                    token0,
                    token1,
                    ..Default::default()
                })
            })
            .collect(),
    })
}
```

We can now take that map module output and direct these pool creation events into a Substreams key-value store using a store module:
```yaml
  - name: store_pools_created
    kind: store
    updatePolicy: set
    valueType: proto:uniswap.types.v1.Pool
    inputs:
      - map: map_pools_created
```
```rust
#[substreams::handlers::store]
pub fn store_pools_created(pools: Pools, store: StoreSetProto<Pool>) {
    for pool in pools.pools {
        let pool_address = &pool.address;
        store.set(pool.log_ordinal, format!("pool:{pool_address}"), &pool);
    }
}
```

Above we are using `pool:{pool_address}` as a key to store the pool information. Eventually, our store will contain all Uniswap pools.
Now, in the downstream modules, we can easily retrieve our pool from the store whenever we need it.

```yaml
- name: map_events
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
      - store: store_pools_created
    output:
      type: proto:uniswap.types.v1.Events
```

```rust
#[substreams::handlers::map]
pub fn map_events(block: Block, pools_store: StoreGetProto<Pool>) -> Result<Events, Error> {
    let mut events = Events::default();

    for trx in block.transactions() {
        for (log, call_view) in trx.logs_with_calls() {
            let pool_address = &Hex(&log.address).to_string();

            let pool = match pools_store.get_last(format!("pool:{pool_address}")) {
                Some(pool) => pool,
                None => { continue; }
            };

            // use the pool information from the store
        }
    }

    Ok(events)
}
```

Here we use `pools_store.get_last()` method to get the pool from the store by its smart contract address. Once we have it, we can use that information to analyze the swap transaction and emit the events.

Alternatively, we could make RPC calls to get the pool details from an RPC node but that would be extremely inefficient considering that we would need to make RPC calls for millions of such events. Using a store will be much faster.

For a real-life application of this pattern see [Uniswap V3 Substreams](https://github.com/streamingfast/substreams-uniswap-v3)


## Links
* [Uniswap-v3 Subgraph and Substreams](https://github.com/streamingfast/substreams-uniswap-v3)
* [Substreams Sink Entity Changes](https://github.com/streamingfast/substreams-sink-entity-changes)