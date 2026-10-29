---
description: Skip blocks that hold nothing of interest with block filters and index modules
---

# Block Filters

A block filter lets a module run only on the blocks that matter to it. It pairs a module with an [index module](indexes.md) (`kind: blockIndex`), which emits a list of keys for every block, and a query matched against those keys. On a block where the query does not match, the module is not executed.

{% hint style="success" %}
**Use an existing foundational index module.** The foundational packages provide index modules for the common cases, listed in [Foundational index modules](#foundational-index-modules). Their index is computed once for the whole chain and is likely already cached on the server, so your filter costs nothing to set up.

Writing your own index module only pays off when several of your modules filter on it. The index module runs on every block, so for a single module it costs about as much as checking the condition in the module's own code.
{% endhint %}

## How a block filter works

This module reads the events of every block from `ethereum_common`, but only runs on blocks where the USDC contract emitted an event:

{% code title="substreams.yaml" %}
```yaml
imports:
  ethereum_common: ethereum_common@v0.3.3

modules:
  - name: map_usdc_events
    kind: map
    inputs:
      - map: ethereum_common:all_events
    blockFilter:
      module: ethereum_common:index_events
      query:
        string: evt_addr:0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48
    output:
      type: proto:my.types.v1.Events
```
{% endcode %}

1. `ethereum_common:index_events` emits keys for every block, such as `evt_addr:0xa0b8...` for every contract that emitted an event in the block.
2. The server stores these keys as an index: for each key, the list of blocks that contain it.
3. Before running `map_usdc_events` on a block, the server checks the query against the keys of that block. When the query does not match, `map_usdc_events` is skipped and its output for that block is empty.

The index module is computed once per block, then read from the cache by every request and every module that filters on it, including the modules of other users. When the index is already cached for a range of blocks, the server evaluates the query against the stored index for the whole range before processing it, and knows in advance which blocks to run the module on.

## Foundational index modules

These index modules are available in the foundational packages:

* `ethereum_common` (for example `ethereum_common@v0.3.3`), for Ethereum and EVM chains:
  * `index_events`: `evt_addr:` (address of the contract that emitted the event) and `evt_sig:` (event signature, `topic0`).
  * `index_calls`: `call_to:` (called contract), `call_from:` (caller), and `call_method:` (first 4 bytes of the call data).
  * `index_events_and_calls`: all the keys of `index_events` and `index_calls`.
* `solana_common` (for example `solana_common@v0.3.3`), for Solana, ignoring vote transactions:
  * `program_ids_without_votes`: `program:` (program ID).
  * `program_ids_and_accounts_without_votes`: `program:` and `account:`.

Addresses and signatures are lowercase hex, prefixed by `0x`. Run `substreams info ethereum_common@v0.3.3` to see the documentation of each module.

## What a block filter saves

On a skipped block:

* The module's code is not executed, so there is no decoding of its inputs and no processing.
* A module that reads only from skipped modules, with no `source` input, is skipped as well.
* During backfill, nothing is written to the module's cache and nothing is sent to you. Near the chain head, you receive an empty output for the block, so you can follow progress.

When the cached index shows that no block of a segment (a range of blocks processed as one unit during backfill) matches, the server skips the segment entirely: it does not load the stores the module depends on and does not load the WASM code.

The index module itself is not free: the first time it runs, it processes every block from its initial block. Its output is then cached and shared, so this cost is paid once per index module, and not once per query. For a foundational index module, it is likely already paid. The storage of index files is not metered.

## Configuring a block filter

The `blockFilter` field has two parts:

* `module`: the name of a module of `kind: blockIndex`. It is usually a [foundational index module](#foundational-index-modules), such as `solana:program_ids_without_votes`, but it can also be one of your own. Its `initialBlock` cannot be greater than the `initialBlock` of the filtered module.
* `query`: exactly one of:
  * `string`: a fixed query.
  * `params: true`: use the value of the module's `params` input as the query. The module must have a `params` input. This lets the consumer of your package choose the filter at run time with `-p`, or under `params` in a manifest that imports yours.

{% code title="substreams.yaml" %}
```yaml
imports:
  solana: solana_common@v0.3.3

modules:
  - name: map_program_data
    kind: map
    inputs:
      - params: string
      - source: sf.solana.type.v1.Block
    blockFilter:
      module: solana:program_ids_without_votes
      query:
        params: true
    output:
      type: proto:my.program.v1.Data

params:
  map_program_data: program:6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P
```
{% endcode %}

Both `map` and `store` modules can have a block filter. A `blockIndex` module cannot.

A module declared with [`use`](../manifests.md#module-use) inherits the block filter of the module it uses. Set `blockFilter: {}` to run it without a filter.

## Query syntax

A query matches the keys of a block:

* `key` matches blocks that have the key, for example `program:6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P`.
* `a && b`, or `a b`, matches blocks that have both keys, for example `evt_addr:0xa0b8 && evt_sig:0xddf2`.
* `a || b` matches blocks that have at least one of the keys, for example `evt_addr:0xa0b8 || evt_addr:0xdac1`.
* `-a` matches blocks that don't have the key, for example `evt_addr:0xa0b8 -evt_sig:0xddf2`.
* `( )` groups expressions, for example `(evt_addr:0xa0b8 || evt_addr:0xdac1) && evt_sig:0xddf2`.

Keys are matched exactly, so the case of addresses matters. Wrap a key in single or double quotes when it contains spaces or parentheses.

The query is part of the filtered module's [hash](../../module-caching.md): each query has its own cache for the filtered module. The index module's hash does not depend on the query, so all queries share the same index.

## Using block filters well

* **Use a [foundational index module](#foundational-index-modules).** It is likely already cached, and shared with every other user of the package.
* **Filter on selective keys.** The savings come from the blocks that are skipped. A query that matches most blocks saves little.
* **Filter as early as possible.** Put the filter on the first module that only needs matching blocks. Modules that read only from it are skipped too.
* **Use `params: true` for packages others consume.** The consumer then picks the addresses or programs to follow without rebuilding your package.

### Writing your own index module

Write your own index module only when no foundational index has the keys you need, and several of your modules filter on it. See [Indexes](indexes.md) for how to write one, and:

* **Keep it cheap.** An index module runs on every block. Build it from a module you already need, instead of decoding the block a second time.
* **Prefix your keys.** Use a prefix per kind of key, such as `evt_addr:` and `evt_sig:`, so that values of different kinds cannot match each other.
* **Emit only keys you will query.** Don't emit values that are unique to one block, like transaction hashes: they make the index larger without making it more useful.
