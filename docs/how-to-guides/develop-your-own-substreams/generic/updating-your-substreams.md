---
description: Ship a new version of a Substreams package without processing unchanged stores again
---

# Updating your Substreams

Substreams caches the output of every module under its [module hash](../../../references/module-caching.md). When you publish a new version of a package, every module whose hash changed is processed again from its initial block. For a map, that costs only the blocks you request. For a store, it means a full backfill from its `initialBlock` before any module that reads the store can produce output.

This page shows how to update a package while keeping the hash, and the cached data, of the modules whose code did not change.

## What changes a module hash

The module hash is computed from:

* the content and `type` of the binary the module runs. This is the whole WASM file, not only the module's function. Any change to the crate, including in code the module never calls or in a dependency version, changes the hash of every module built from that file.
* the name of the Rust function the module calls
* the module `kind`
* the module `initialBlock`, after it is resolved from the inputs when not set
* the inputs: their order and type, the `source` type, the `params` value, and the `foundational-store` identifier
* the `blockFilter`: the hash of the index module and the query
* the hash of every module the module reads from, directly or indirectly

The module name in the manifest, its `doc`, and the package name and version are not part of the hash.

To compare hashes between two versions, run `substreams info` on each and look at the `Hash:` line of each module:

```bash
substreams info polymarket-v0.1.0.spkg
substreams info substreams.yaml
```

## Two ways to keep a module's hash

To keep a module's hash, run it with the exact binary of the previous version. There are two ways to do this. Both give the same result, so pick one.

The examples on this page use a package with two modules: `store_markets`, which tracks markets, and `map_trades`, which outputs trades and reads `store_markets`.

### Option 1: import the previous package

Import the previous `.spkg`, and declare the module with [`use`](../../../references/substreams-components/manifests.md#module-use):

{% code title="substreams.yaml" %}
```yaml
imports:
  v0_1_0: ./polymarket-v0.1.0.spkg

modules:
  - name: store_markets
    use: v0_1_0:store_markets
    initialBlock: 40000000
```
{% endcode %}

The binary of the previous version is embedded in the `.spkg`, and the modules `store_markets` reads from come along with it. This is the recommended option.

Declaring the module with `use` keeps its name in your package, so the rest of the manifest, `substreams run` commands, and packages that import yours keep referring to `store_markets`. You can also skip `use` and read the imported module directly, for example `store: v0_1_0:store_markets` in `inputs`, which gives the same hash.

### Option 2: keep the previous WASM file

Keep the compiled WASM file of the previous version in your project, declare it as a second binary, and set `binary` on the module:

{% code title="substreams.yaml" %}
```yaml
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/polymarket.wasm
  v0_1_0:
    type: wasm/rust-v1
    file: ./previous/polymarket-v0.1.0.wasm

modules:
  - name: store_markets
    kind: store
    binary: v0_1_0
    initialBlock: 40000000
    updatePolicy: set
    valueType: proto:polymarket.v1.Market
    inputs:
      - source: sf.ethereum.type.v2.Block
```
{% endcode %}

The WASM file must be the exact file of the previous version. Building the previous source code again can produce a different file. Every module the store reads from, directly or indirectly, must also use the previous binary, because their hashes are part of the store's hash.

### Check that the hash is unchanged

With either option:

* **Set `initialBlock` to the value of the previous version.** `use` does not copy `initialBlock`. When it is omitted, it is resolved from the module's inputs, which can give a different value and therefore a different hash.
* **Keep the inputs identical.** With `use`, leave out `inputs` to copy them from the previous version.
* **Keep the `params` value.** The `params` value is part of the hash.
* **Compare with `substreams info`** before you deploy.

The reused module emits the protobuf message of the previous version. If you changed that message, update the code that reads it, or don't reuse the module.

## Updating to fix a bug

Version `v0.1.0` has a bug in `map_trades`, and `store_markets` is correct. The fix changes the Rust code of `map_trades` only, but it changes the WASM file, so without precautions `store_markets` would be processed again from block 40,000,000.

In `v0.1.1`, `store_markets` comes from `v0.1.0`, and only `map_trades` runs the new code:

{% code title="substreams.yaml" %}
```yaml
package:
  name: polymarket
  version: v0.1.1

imports:
  v0_1_0: ./polymarket-v0.1.0.spkg

modules:
  - name: store_markets
    use: v0_1_0:store_markets
    initialBlock: 40000000

  - name: map_trades
    kind: map
    initialBlock: 40000000
    inputs:
      - source: sf.ethereum.type.v2.Block
      - store: store_markets
    output:
      type: proto:polymarket.v1.Trades
```
{% endcode %}

`store_markets` keeps its hash and is read from the cache. `map_trades` gets a new hash and is processed again, which is the point of the fix.

## Updating to support a new protocol version

Suppose that Polymarket deploys a new exchange contract, with a new market format. In this example, the new contract goes live at block 80,000,000, and the previous contract stops being used at block 80,500,000. These numbers are made up for the example.

Version `v0.1.0` is fully processed and cached:

```
store_markets ──> map_trades
```

Version `v0.2.0` adds a store for the new markets, and a new output module that combines the trades of both contracts:

```
v0.1.0, imported:   store_markets ──> map_trades ──────────┐
                                                           ▼
v0.2.0, new code:   store_markets_v2 ─────────────> map_trades_v2
```

The new package only contains the code of `store_markets_v2` and `map_trades_v2`. It imports `map_trades` from `v0.1.0`:

{% code title="substreams.yaml" %}
```yaml
package:
  name: polymarket
  version: v0.2.0

imports:
  v0_1_0: ./polymarket-v0.1.0.spkg

modules:
  - name: store_markets_v2
    kind: store
    initialBlock: 80000000
    updatePolicy: set
    valueType: proto:polymarket.v2.Market
    inputs:
      - source: sf.ethereum.type.v2.Block

  - name: map_trades_v2
    kind: map
    initialBlock: 40000000
    inputs:
      - source: sf.ethereum.type.v2.Block
      - map: v0_1_0:map_trades
      - store: store_markets_v2
    output:
      type: proto:polymarket.v2.Trades
```
{% endcode %}

`map_trades_v2` forwards the trades of `map_trades` until the previous contract stops being used, and adds the trades of the new contract:

{% code title="src/lib.rs" %}
```rust
const V1_END_BLOCK: u64 = 80_500_000;

#[substreams::handlers::map]
fn map_trades_v2(
    block: eth::Block,
    v1_trades: v1::Trades,
    markets: StoreGetProto<v2::Market>,
) -> Result<v2::Trades, Error> {
    let mut trades = v2::Trades::default();

    if block.number < V1_END_BLOCK {
        trades.trades.extend(v1_trades.trades.into_iter().map(convert_v1_trade));
    }
    trades.trades.extend(extract_v2_trades(&block, &markets));

    Ok(trades)
}
```
{% endcode %}

With this layout:

* `store_markets` and `map_trades` run the code of `v0.1.0`, keep their hashes, and are read from the cache.
* `store_markets_v2` starts at block 80,000,000, so its backfill only covers the blocks after the upgrade.
* `map_trades_v2` is new, so it is processed from block 40,000,000. It is a map, so this does not rebuild any store: before block 80,000,000, it only reads the cached output of `map_trades`.
* `map_trades_v2` must set `initialBlock`: its inputs start at different blocks, so it cannot be resolved from them.

Choose `V1_END_BLOCK` before you deploy. Changing it later changes the WASM file, and so the hash of `store_markets_v2`. If you need to change it, reuse `store_markets_v2` from `v0.2.0` with one of the [two options](#two-ways-to-keep-a-modules-hash).

## Structure your project for updates

When you start a project, separate the modules that are expensive to process again from the modules you change often:

* Put stable stores, and the modules they read from, in their own package and crate.
* Import that package from the package that holds the modules you iterate on, such as `db_out` or filtering maps.

A change to the second package then never changes the WASM file of the first one. See [Composing Substreams](../../composing-substreams/composing-substreams.md) and [Module Caching](../../../references/module-caching.md#designing-for-efficient-store-caching).
