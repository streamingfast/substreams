---
description: StreamingFast Substreams modules overview
---

# Modules Overview

## What are Modules?

Modules are a crucial aspect of working with Substreams. Modules provide hooks into the execution of the Substreams compute engine. Developers will craft their own data manipulation and transformation strategies within modules.

In further detail, modules are small pieces of Rust code running in a WebAssembly (WASM) virtual machine. Modules coexist within the stream of blocks sent by the Substreams compute engine arriving from a blockchain node.&#x20;

Modules have one or more inputs. The inputs can be in the form of a `map` or `store,` or a `Block` or `Clock` received from the blockchain's data source.

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
Substreams modules data interaction diagram
{% endembed %}

In the diagram shown above the `transfer_map` module extracts all transfers in each `Block,` and the `transfer_counter` store module tracks the number of transfers that have occurred.

{% hint style="info" %}
**Note:** Multiple inputs are made possible because blockchains are clocked.&#x20;

Blockchains allow synchronization between multiple execution streams opening up great performance improvements over comparable traditional streaming engines.
{% endhint %}

Modules can also take in multiple inputs as seen in the `counters` store example diagram below. Two modules feed into a `store`, effectively tracking multiple `counters`.

{% embed url="https://mermaid.ink/svg/pako:eNqdkE1qAzEMha9itE4GsnWgi5KcINmNh6LamozJeGxsuSGE3L1KW1PIptCdnnjv088NbHQEGk4Z06SOu61ZlHqfoz33JdZsSasydsQTZaqh42ui7mPTvT4cg1qvX1TA9HbxPLmMF5zLv_KOUiyev8JPvF60fm5-J22sC1MufeGYZVDTQ8M07C-jdf4AwAoC5YDeyWtuD5wBOSGQAS2loxHrzAbMchdrTQ6Z9s4LBfQo-9EKsHI8XBcLmnOlZtp5lE-HH9f9EylZic0" %}
Modules with multiple inputs diagram
{% endembed %}

All of the modules are executed as a directed acyclic graph (DAG) each time a new `Block` is processed.

{% hint style="info" %}
**Note:** The top-level data source is always a protocol's `Block` protobuf model, and is deterministic in its execution.
{% endhint %}

## Single Output

Modules have a _**single typed output.**_ Modules are typed to inform consumers of the types of data to expect and also how to interpret the bytes being sent.

{% hint style="success" %}
**Tip**: Data that is output from one module is used as the input for subsequent modules basically forming a daisy chain of data flow from module to module.
{% endhint %}

## Modules: Map vs. Store

Most non-trivial Substreams development initiatives will require the use of both map and store modules and more than one of each. The exact number, responsibilities, and how modules communicate will depend on many factors specific to the developer’s desired, final Substreams solution.

The two module types are commonly used together to construct the directed acyclic graph (DAG) outlined in the Substreams manifest. Map and store modules are very different in their use and how they work. Understanding these differences is important for harnessing the full power of Substreams.

### Map Modules

Map modules are used for data extraction, filtering, and transformation. They should be used when direct extraction is needed avoiding the need to reuse them later in the DAG.

For performance considerations, developers should use a single map, instead of multiple maps that extract single events/functions. It's better to perform as much extraction as possible from a singular, top-level map module and then pass the data around for consumption by other Substreams modules. This is the most straightforward, simplistic, and recommended approach for both the backend and consumer development experience.

Notable facts and use cases for working map modules include:

- Extracting model data from an event or a function's inputs.
- Reading data from a block and transforming said data into a custom protobuf structure.
- Filtering out events or functions on any given number of contracts.

### Store Modules

Store modules are used for aggregation of values and to temporarily persist state that exists across a block.

{% hint style="info" %}
**Note:** Stores should not be used for temporary, free-form data persistence.
{% endhint %}

Unbounded stores are discouraged. Stores shouldn't be used as an infinite bucket to dump data into.

Notable facts and use cases for working store modules include:

- Stores should only be used when reading data from another downstream Substreams module.
- Stores cannot be outputted as a stream, except in development mode.
- Stores are used to implement the Dynamic Data Sources pattern from Subgraphs; keeping track of contracts created to filter the next block with that information.
- Do not use stores to query anything from them downstream of the Substreams output; use a sink and shape the data for proper querying.

## Next steps

Read more about [modules in the developer guide](../developer-guide/modules/).

####
