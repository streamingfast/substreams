# Modules

Modules are small pieces of code, running in a WebAssembly virtual machine, amidst the stream of blocks arriving from a blockchain name, with a blockchain network's history waiting to be processed in flat files.

The top-level source of a module tree will be blockchain data, in the form of Firehose Blocks for each supported blockchain protocol. See the [Firehose documentation](http://firehose.streamingfast.io/) for more details.

Modules may have one or more inputs (from multiple modules, be them `map`s or `store`s, and/or from the blockchain's data in the form of a _Block_).

> Multiple inputs are made possible because blockchains have a clock, and allows synchronization between multiple execution streams, opening up great performance improvements even over your comparable traditional streaming engine.

Modules have a single output, that can be typed, to inform consumers what to expect and how to interpret the bytes coming out.

There are two types of modules, a `map` module, and a `store` module.

Modules can form a graph of modules, taking each other's output as the next module's input, like so:

{% embed url="https://mermaid.ink/svg/pako:eNp1kM0KwjAQhF8l7NkWvEbwIPUJ9NYUWZKtLTZJ2WwEEd_dCAr-4GFhd_h2GOYKNjoCDUfGeVD7ZmWCUqmvSQZiyr6Wy0z1eVlvpmhPbYqZLen_RKeqaq2EMaSe-OBxfhi-320Z_aF8_diYgxC3SSKT_tE7WIAn9ji6kvv6sDdQsngyoMvqqMc8iQETbgXNs0OhrRuLG-gep0QLwCxxdwkWtHCmF9SMWGrwT-p2B02rZZY" %}
\[To edit, open link and replace "ink/svg/" by "live/edit#"]
{% endembed %}

Here, the `transfer_map` module would extract all transfers that happened in each block,

## A `map` module

A `map` module takes bytes in, and outputs bytes. In the [manifest](../reference/manifest.md), you would declare the protobuf types to help users decode the streams, and help generate some code to get you off the ground faster.

## A `store` module

A `store` module is slightly different in that it is a _stateful_ module. It contains a _key/value_ store that can be either _written_ to, or read from.

The code you provide when designing a `store` module can only _write_ to the key/value store in particular ways. See the [API Reference](../reference/api-reference.md) for more details on store semantics.

When consuming a store (when it is set as a dependency to a module that depends on this store), you can only _read_ from it.

See API Reference

\[TODO: give example, explain some of the playground examples]
