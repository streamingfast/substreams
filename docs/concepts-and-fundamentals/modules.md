---
description: StreamingFast Substreams modules overview
---

# Modules basics

## Modules overview

Modules are an important part of Substreams, offering hooks into the execution of the Substreams compute engine. You can create Substreams data manipulation and transformation strategies within modules.

Modules are small pieces of Rust code running in a [WebAssembly (WASM)](https://webassembly.org/) virtual machine. They coexist within the stream of blocks sent by the Substreams compute engine, which arrives from a blockchain node.

Modules have one or more inputs, which can be in the form of a `map` or `store`, or a `Block` or `Clock` object received from the blockchain's data source.

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
Substreams modules data interaction diagram
{% endembed %}

The diagram shows how the `transfer_map` module extracts the transfers in a `Block` and tracks the total number of transfers.

{% hint style="info" %}
**Note:** Because blockchains are clocked, you can use multiple inputs. This allows synchronization between multiple execution streams, resulting in improved performance over comparable conventional streaming engines.
{% endhint %}

As seen in the counters store example diagram, modules can also take in multiple inputs. In this case, two modules feed into a `store`, effectively tracking multiple `counters`.

{% embed url="https://mermaid.ink/svg/pako:eNqdkE1qAzEMha9itE4GsnWgi5KcINmNh6LamozJeGxsuSGE3L1KW1PIptCdnnjv088NbHQEGk4Z06SOu61ZlHqfoz33JdZsSasydsQTZaqh42ui7mPTvT4cg1qvX1TA9HbxPLmMF5zLv_KOUiyev8JPvF60fm5-J22sC1MufeGYZVDTQ8M07C-jdf4AwAoC5YDeyWtuD5wBOSGQAS2loxHrzAbMchdrTQ6Z9s4LBfQo-9EKsHI8XBcLmnOlZtp5lE-HH9f9EylZic0" %}
Multiple module inputs diagram
{% endembed %}

Every time a new `Block` is processed, all of the modules are executed as a directed acyclic graph (DAG).

{% hint style="info" %}
**Note:** The top-level data source is always a protocol's `Block` protobuf model, which is deterministic in its execution.
{% endhint %}

## Single Output

Modules have a single typed output, which is typed to inform consumers of the types of data to expect and how to interpret the bytes being sent.

{% hint style="success" %}
**Tip**: Subsequent modules use data output from one module as input, forming a chain of data flow from module to module.
{% endhint %}

## Next steps

Learn more about [modules in the Developer's guide](../developers-guide/modules/).

####
