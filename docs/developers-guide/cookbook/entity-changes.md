---
description: Creating and Updating Subgraph entities
---

# Creating and Updating Subgraph entities
A common convention for Substreams-powered subgraph development is to implement a `graph_out` module emitting `sf.substreams.sink.entity.v1.EntityChanges` events. When these events are consumed by the indexer, entities in the subgraph are updated accordingly.

[substreams-entity-change](https://crates.io/crates/substreams-entity-change) crate offers helper methods to assist with that.

Here's a sample declaration of `graph_out` module in `substreams.yaml`:

```yaml
 - name: graph_out
    kind: map
    inputs:
      - source: sf.substreams.v1.Clock
      - store: store_eth_prices
        mode: deltas
    output:
      type: proto:sf.substreams.sink.entity.v1.EntityChanges
```

This module is for a simple subgraph that tracks ETH/USD rate derived from Uniswap pools.

`Clock` input source emits events every block and helps you keep track of the block number and timestamp.

`store_eth_prices` module stores derived ETH prices and in `delta` mode it 

Our subgraph has a single entity:
```graphql
# stores for USD calculations
type Bundle @entity {
  id: ID!
  # price of ETH in usd
  ethPriceUSD: BigDecimal!
}
```

Corresponding Rust code for the `graph_out` module can look like this:

```rust
use substreams_entity_change::pb::entity::EntityChanges;
use substreams_entity_change::tables::Tables;

#[substreams::handlers::map]
pub fn graph_out(clock: Clock, derived_eth_prices_deltas: Deltas<DeltaBigDecimal>) -> Result<EntityChanges, Error> {
    let mut tables = Tables::new();

    if clock.number == 12369621 {
        tables
            .create_row("Bundle", "1")
            .set("ethPriceUSD", BigDecimal::zero());
    }

    for delta in derived_eth_prices_deltas.into_iter(){
        tables.update_row("Bundle", "1").set("ethPriceUSD", delta.new_value);
    }

    Ok(tables.to_entity_changes())
}

```
Let's break it down.

`substreams-entity-change` crate offers `Tables` struct to work with the entities.
First, we instantiate `Tables` object:
```rust
let mut tables = Tables::new();
```

Then we check if this is the first block of our substream and if so, we create the entity using `create_row` method.
```rust
if clock.number == 12369621 {
    tables
        .create_row("Bundle", "1")
        .set("ethPriceUSD", BigDecimal::zero());
}
```
`create_row` takes two arguments: entity name and entity id. In our case, we use "Bundle" entity name - that's the entity we have defined in the subgraph `schema.graphql` schema. We use "1" as `id`. That's the only price that we will have.

Note: `12369621` magic block number is used here for simplicity. Typically you would define it as a module parameter.

Next, we iterate through all ETH price deltas within that block and update it in our table.
```rust
for delta in derived_eth_prices_deltas.into_iter(){
    tables
        .update_row("Bundle", "1")
        .set("ethPriceUSD", delta.new_value);
}
```
Here, we use `update_row` method to create an `UPDATE` entity operation, and we use `set` method to set the corresponding entity field.

One last step, we convert our `Tables` helper object into the `EntityChanges` object that our subgraph can consume:
```rust
Ok(tables.to_entity_changes())
```

## Links
* [Uniswap-v3 Subgraph and Substreams](https://github.com/streamingfast/substreams-uniswap-v3)
* [Substreams Entity Changes](https://github.com/streamingfast/substreams-sink-entity-changes)