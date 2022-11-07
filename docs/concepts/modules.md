---
description: StreamingFast Substreams modules overview
---

# Modules Overview

### What are Modules?

Modules are a crucial aspect of working with Substreams. The modules provide hooks into the execution of the Substreams compute engine. Developers will craft their own data manipulation and transformation strategies into modules contained.

In further detail, modules are small pieces of Rust code running in a WebAssembly (WASM) virtual machine. Modules coexist within the stream of blocks sent by the Substreams compute engine arriving from a blockchain node.&#x20;

{% hint style="success" %}
**Tip**: Modules can also process network history from flat files backed by StreamingFast Firehose. See [Firehose documentation](http://firehose.streamingfast.io/) for additional information.
{% endhint %}

Modules have one or more inputs. The inputs can be in the form of a `map` or `store,` or a `Block` or `Clock` received from the blockchain's data source.

{% hint style="info" %}
**Note:** Multiple inputs are made possible because blockchains are clocked.&#x20;

Blockchains allow synchronization between multiple execution streams opening up great performance improvements over comparable traditional streaming engines.
{% endhint %}

### Single Output

{% hint style="info" %}
**Note**: Modules have a _**single output.**_ The output is generally typed informing consumers what to expect, and how to interpret the bytes being sent from the module.
{% endhint %}

Modules are typically formed into a graph; a directed acyclic graph (DAG) to be specific.&#x20;

{% hint style="success" %}
**Tip**: Data that is output from one module is used as the input for subsequent modules basically forming a daisy chain of data flow from module to module.
{% endhint %}

#### Modules Example

In the diagram shown below the `transfer_map` module extracts all transfers in each `Block,` and the  `transfer_counter` store module tracks the number of transfers that have occurred.

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
Substreams modules data interaction diagram
{% endembed %}

Modules can also take in multiple inputs as seen in the `counters` store example diagram below. Two modules feed into a `store`, effectively tracking multiple `counters`.

{% embed url="https://mermaid.ink/svg/pako:eNqdkE1qAzEMha9itE4GsnWgi5KcINmNh6LamozJeGxsuSGE3L1KW1PIptCdnnjv088NbHQEGk4Z06SOu61ZlHqfoz33JdZsSasydsQTZaqh42ui7mPTvT4cg1qvX1TA9HbxPLmMF5zLv_KOUiyev8JPvF60fm5-J22sC1MufeGYZVDTQ8M07C-jdf4AwAoC5YDeyWtuD5wBOSGQAS2loxHrzAbMchdrTQ6Z9s4LBfQo-9EKsHI8XBcLmnOlZtp5lE-HH9f9EylZic0" %}
Modules with multiple inputs diagram
{% endembed %}

All of the modules are executed as a directed acyclic graph (DAG) each time a new `Block` is processed.

{% hint style="info" %}
**Note:** The top-level data source is always a protocol's `Block` protobuf model, and is deterministic in its execution.
{% endhint %}
