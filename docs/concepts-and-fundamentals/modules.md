---
description: StreamingFast Substreams modules overview
---

# Modules overview

## What are modules?

Modules are a crucial aspect of working with Substreams. Modules provide hooks into the execution of the Substreams compute engine. Developers will craft their own data manipulation and transformation strategies within modules.

In further detail, modules are small pieces of Rust code running in a WebAssembly (WASM) virtual machine. Modules coexist within the stream of blocks sent by the Substreams compute engine arriving from a blockchain node.&#x20;

Modules have one or more inputs. The inputs can be in the form of a `map` or `store,` or a `Block` or `Clock` received from the blockchain's data source.

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
Substreams modules data interaction diagram
{% endembed %}

The diagram shows the `transfer_map` module extracts all transfers in each `Block,` and the  `transfer_counter` store module tracks the number of transfers that have occurred.

{% hint style="info" %}
**Note:** You can use multiple inputs because blockchains are clocked.&#x20;

Blockchains allow synchronization between multiple execution streams opening up great performance improvements over comparable conventional streaming engines.
{% endhint %}

Modules can also take in multiple inputs as seen in the `counters` store example diagram. Two modules feed into a `store`, effectively tracking multiple `counters`.

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

## Next steps

Read more about [modules in the Developer's Guide](../developers-guide/modules/).

####
