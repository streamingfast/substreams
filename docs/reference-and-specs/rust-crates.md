---
description: StreamingFast Substreams Rust APIs
---

# Rust crates

### Substreams crates

The official [`substreams` crate](https://crates.io/crates/substreams) assists developers with creating module handlers.

There are also chain-specific `substreams-[network]` crates available:

* [`substreams-ethereum`](https://crates.io/crates/substreams-ethereum) for Ethereum and other Ethereum-compatible chains
* [`substreams-solana`](https://crates.io/crates/substreams-solana) for Solana

For Substreams where no crate is available, you can use the `spkg` released for the chain, which contains the Block protobuf model, and generate the Rust structs yourself:

```bash
```

### Third-party libraries

Any third-party library capable of compiling `wasm32` can be used for execution in Substreams services.&#x20;

Many libraries compile kernel `syscalls`, or other operations, which are not available within the Substreams execution environment and will not successfully compile to `wasm32` targets.

Here's a very inexhaustive list of things people found useful:

* [`tiny_keccak`](https://docs.rs/tiny-keccak): an implementation of Keccak derived functions specified in FIPS-202, SP800-185 and KangarooTwelve.
