---
description: Ship a new version of a Substreams package without processing unchanged stores again
---

# Updating your Substreams

Substreams caches the output of every module under its [module hash](../references/module-caching.md). When you publish a new version of a package, every module whose hash changed is processed again from its initial block. For a map, that costs only the blocks you request. For a store, it means a full backfill from its `initialBlock` before any module that reads the store can produce output.

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
substreams info my-package-v0.1.0.spkg
substreams info substreams.yaml
```

## Reuse modules from a previous version

To keep the hash of a module whose code did not change, run it from the previously published package instead of from your new binary. Import the previous version, and declare the module with [`use`](../references/substreams-components/manifests.md#module-use):

{% code title="substreams.yaml" %}
```yaml
package:
  name: my_package
  version: v0.2.0

imports:
  previous: ./my-package-v0.1.0.spkg

modules:
  - name: map_events
    use: previous:map_events
    initialBlock: 12287507

  - name: store_totals
    use: previous:store_totals
    initialBlock: 12287507

  - name: db_out
    kind: map
    inputs:
      - map: map_events
      - store: store_totals
        mode: deltas
    output:
      type: proto:sf.substreams.sink.database.v1.DatabaseChanges
```
{% endcode %}

In this example, only `db_out` runs code from the new binary. `map_events` and `store_totals` run the code of `my-package-v0.1.0.spkg`, which is embedded in the new package, so their hashes stay the same and the server reuses their cached data.

Keep these rules in mind:

* **Set `initialBlock` to the value of the previous version.** `use` does not copy `initialBlock`. When it is omitted, it is resolved from the module's inputs, which can give a different value and therefore a different hash.
* **Leave out `inputs`, or keep them identical.** When `inputs` is omitted, the module reads from the same modules as in the previous version, which keeps the hash of the whole chain.
* **Keep the `params` value.** The `params` value is part of the hash.
* **The output type is the previous one.** The reused module emits the protobuf message of the previous version. If you changed that message in your new `.proto` files, update the code that reads it, or don't reuse the module.

Declaring the module with `use` keeps its name in your package. You can also read an imported module directly, for example `store: previous:store_totals` in `inputs`, which gives the same hash. With `use`, the `sink` section, `params` and `networks` keys, `substreams run` commands, and packages that import yours keep referring to `store_totals`.

## Keep the previous binary in your project

Instead of importing the previous package, you can keep its compiled WASM file in your project, and declare it as a second binary:

{% code title="substreams.yaml" %}
```yaml
binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/my_package.wasm
  v0_1_0:
    type: wasm/rust-v1
    file: ./previous/my_package-v0.1.0.wasm

modules:
  - name: map_events
    kind: map
    binary: v0_1_0
    ...
  - name: store_totals
    kind: store
    binary: v0_1_0
    ...
```
{% endcode %}

Every module that the store reads from, directly or indirectly, must also use the previous binary, because their hashes are part of the store's hash. Importing the previous package handles this for you, and is the recommended approach.

## Structure your project for updates

When you start a project, separate the modules that are expensive to process again from the modules you change often:

* Put stable stores, and the modules they read from, in their own package and crate.
* Import that package from the package that holds the modules you iterate on, such as `db_out` or filtering maps.

A change to the second package then never changes the WASM file of the first one. See [Composing Substreams](composing-substreams/composing-substreams.md) and [Module Caching](../references/module-caching.md#designing-for-efficient-store-caching).
