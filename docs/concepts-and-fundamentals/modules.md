---
description: StreamingFast Substreams modules basics
---

# Modules basics

## Modules basics overview

Modules are an important part of Substreams, offering hooks into the execution of the Substreams compute engine. You can create Substreams data manipulation and transformation strategies within modules.

Modules are small pieces of Rust code running in a [WebAssembly (WASM)](https://webassembly.org/) virtual machine. They coexist within the stream of blocks sent by the Substreams compute engine, which arrives from a blockchain node.

Modules have one or more inputs, which can be in the form of a `map` or `store`, or a `Block` or `Clock` object received from the blockchain's data source.

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
Substreams modules data interaction diagram
{% endembed %}

The diagram shows how the `transfer_map` module extracts the transfers in a `Block` and tracks the total number of transfers.

{% hint style="info" %}
**Note:** You can use multiple inputs in blockchains because they are clocked, which allows for synchronization between multiple execution streams and improved performance compared to conventional streaming engines.
{% endhint %}

As seen in the `counters` `store` example diagram, modules can also take in multiple inputs. In this case, two modules feed into a `store`, effectively tracking multiple `counters`.

{% embed url="https://mermaid.ink/svg/pako:eNqdkE1qAzEMha9itE4GsnWgi5KcINmNh6LamozJeGxsuSGE3L1KW1PIptCdnnjv088NbHQEGk4Z06SOu61ZlHqfoz33JdZsSasydsQTZaqh42ui7mPTvT4cg1qvX1TA9HbxPLmMF5zLv_KOUiyev8JPvF60fm5-J22sC1MufeGYZVDTQ8M07C-jdf4AwAoC5YDeyWtuD5wBOSGQAS2loxHrzAbMchdrTQ6Z9s4LBfQo-9EKsHI8XBcLmnOlZtp5lE-HH9f9EylZic0" %}
Multiple module inputs diagram
{% endembed %}

Every time a new `Block` is processed, all of the modules are executed as a directed acyclic graph (DAG).

{% hint style="info" %}
**Note:** The protocol's Block protobuf model always serves as the top-level data source and executes deterministically.
{% endhint %}

### Single output

Modules have a single typed output, which is typed to inform consumers of the types of data to expect and how to interpret the bytes being sent.

{% hint style="success" %}
**Tip**: In subsequent modules, input from one module's data output is used to form a chain of data flow from module to module.
{% endhint %}

### `map` versus `store` modules

To develop most non-trivial Substreams, you will need to use multiple `map` and `store` modules. The specific number, responsibilities, and communication methods for these modules will depend on the developer's specific goals for the Substreams development effort.

The two module types are commonly used together to construct the directed acyclic graph (DAG) outlined in the Substreams manifest. The two module types are very different in their use and how they work. Understanding these differences is vital for harnessing the full power of Substreams.

### `map` modules

`map` modules are used for data extraction, filtering, and transformation. They should be used when direct extraction is needed avoiding the need to reuse them later in the DAG.

To optimize performance, you should use a single `map` module instead of multiple `map` modules to extract single events or functions. It is more efficient to perform the maximum amount of extraction in a single top-level `map` module and then pass the data to other Substreams modules for consumption. This is the recommended, simplest approach for both backend and consumer development experiences.

Functional `map` modules have several important use cases and facts to consider, including:

* Extracting model data from an event or function's inputs.
* Reading data from a block and transforming it into a custom protobuf structure.
* Filtering out events or functions for any given number of contracts.

### `store` modules

`store` modules are used for the aggregation of values and to persist state that temporarily exists across a block.

{% hint style="warning" %}
**Important:** Stores should not be used for temporary, free-form data persistence.
{% endhint %}

Unbounded `store` modules are discouraged. `store` modules shouldn't be used as an infinite bucket to dump data into.

Notable facts and use cases for working `store` modules include:

* `store` modules should only be used when reading data from another downstream Substreams module.
* `store` modules cannot be output as a stream, except in development mode.
* `store` modules are used to implement the Dynamic Data Sources pattern from Subgraphs, keeping track of contracts created to filter the next block with that information.
* Downstream of the Substreams output, do not use `store` modules to query anything from them. Instead, use a sink to shape the data for proper querying.

### Additional information

Learn more about [modules in the Developer's guide](../developers-guide/modules/).
