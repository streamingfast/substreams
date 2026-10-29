---
description: Design Substreams modules that process blocks quickly and cheaply
---

# Performance Guidelines

During a backfill, the time Substreams spends on your package goes to four things: reading blocks, running your modules' code, writing and reading the cached output of every module, and building stores. Each guideline on this page reduces one of them.

## Keep the number of modules small

Every module adds work on every block it processes:

* Its inputs are copied into the WASM engine and decoded by your code.
* Its output is encoded, written to the cache, and read back by every module that consumes it.

A pipeline with five modules does this five times per block, even if each module does very little.

Put all the extraction logic in one module that reads the block, rather than one module per event, call, or instruction. Split a module only when its output is read by several of your other modules.

{% hint style="warning" %}
**Don't create small modules for the sake of composability.** A module that is not reused by other modules adds serialization and cache I/O on every block, and makes nothing reusable. Composability comes from publishing a package with a few useful outputs, not from splitting your logic into many modules.
{% endhint %}

## Reduce the size of your data early

The block is the largest input a module can receive. Read it in as few modules as possible, ideally one, and output only the fields you need.

Every module downstream then reads the smaller output from the cache. A small output is faster to write, faster to read, and faster to decode in each consumer.

When a module has nothing to output for a block, call `substreams::skip_empty_output()` in its code:

```rust
#[substreams::handlers::map]
fn map_trades(block: Block) -> Result<Trades, Error> {
    substreams::skip_empty_output();

    let trades = extract_trades(&block);
    Ok(Trades { trades })
}
```

When the output is empty, it is marked as skipped, and a module whose inputs are all skipped is not executed for that block. Only the clock input is ignored in this check. A protobuf message that has only default values encodes to zero bytes, so it counts as empty.

## Keep stores few and independent

Stores are built in stages during parallel processing. A store that reads from another store, directly or through a map, is built in a later stage than that store:

* Each stage is an additional full pass over the blocks of every segment.
* A segment of the next stage starts only when the store of the previous stage is merged up to that segment, and merging is sequential.

Use as few stores as possible, and avoid chains of stores. When a store only needs data that a map already extracts, read that map instead of another store.

Keep each store small. Every segment loads the full store at its start, so a large store slows down every segment, and stores have a size limit. Delete the keys you no longer need. For large datasets, such as all the token metadata of a chain, use a [Foundational Store](../how-to-guides/composing-substreams/foundational-stores/foundational-stores.md).

Before you update a package, read [Updating your Substreams](../how-to-guides/develop-your-own-substreams/generic/updating-your-substreams.md): a code change gives a new hash to every store built from the same binary, and each one is backfilled again from its initial block.

## Use block filters with foundational indexes

A [block filter](substreams-components/modules/block-filters.md) skips the blocks that hold nothing of interest. When the index it uses is already cached, the server knows in advance which blocks to process, and skips whole segments that have no match.

Use the index modules of the foundational packages, which are shared by all their users. Writing your own index module is only worth it when several of your modules filter on it.

## Set `initialBlock` as late as possible

Set `initialBlock` to the first block that can hold data for your module, for example the block where the contract or program you follow was deployed. Every block before it is skipped. This matters most for stores, which are always backfilled from their initial block.

## Write efficient Rust code

* **Borrow, don't clone.** Iterate over references to the block's transactions, logs, or instructions. Cloning a transaction, or the block, copies all of its content.
* **Check before decoding.** Compare the raw address, event signature, or program ID before decoding a log or instruction. Decoding is the expensive part, and most of the block does not match.
* **Keep data as bytes until the end.** Hex and base58 encoding allocate a string for every value, and hex doubles the size of the data. Encode values only in the last module, for example `db_out`. Base58 encoding is especially slow.
* **Don't log in loops.** Each `substreams::log` call formats a string and copies it out of the WASM engine, even when nobody reads it.
* **Use `int64` and `float64` stores when the values fit.** `bigint` and `bigdecimal` values cost more to encode, decode, and add.

## Chain-specific guidelines

### EVM chains

Start your package from the full block, `sf.ethereum.type.v2.Block`, with a block filter on a foundational index module, such as `index_events` or `index_calls` from `ethereum_common`:

```yaml
imports:
  ethereum_common: ethereum_common@v0.3.3

modules:
  - name: map_trades
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    blockFilter:
      module: ethereum_common:index_events
      query:
        string: evt_addr:0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48
    output:
      type: proto:my.types.v1.Trades
```

Don't read the output of a foundational map, like `ethereum_common:all_events`, assuming it is cached: when it is not, your package processes it for every block in addition to your own module.

Each `eth_call` is a network request to a node, made while the block is processed. The block cannot complete until the call returns, so calls slow down processing more than any computation in your module:

* Make a call only when you need its result, for example the first time you see a token, not on every transfer.
* Batch the calls of a block with `RpcBatch`. See [Contract Calls](../how-to-guides/develop-your-own-substreams/evm/eth-calls.md#batching-calls).
* Read data that does not change, like token decimals, from a Foundational Store such as [ERC20 Token Metadata](../how-to-guides/composing-substreams/foundational-stores/ethereum/erc20-token-metadata.md) instead of calling the contract.

### Solana

Read `sf.solana.type.v1.Block` directly. StreamingFast's Solana endpoints no longer serve vote transactions, so on those endpoints `blocks_without_votes` only adds a module to your pipeline.

Filter on program IDs with the `program_ids_without_votes` index module of `solana_common`, and check the program ID of an instruction before decoding its data.

## Measure

* Measure in production mode. Development mode never reads or writes the cache, so it does not show how your package behaves on a backfill.
* Compare runs in the same cache state. The first run of a module is always slower than the next ones. See [Module Caching](module-caching.md#performance-testing-considerations).
* Benchmark a module's code on heavy blocks. See [Testing](testing.md#performance-testing).

How you consume the output also affects speed. See [RPC Protocol](rpc-protocol.md#performance-considerations) for client-side settings, and the [SQL sink reference](sql/proto-annotations.md#performance-flags) for database sinks.
