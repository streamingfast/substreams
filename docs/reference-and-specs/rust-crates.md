---
description: Substreams Rust APIs
---

# Rust crates

## Rust crates overview

The official [`substreams` crate](https://crates.io/crates/substreams) helps developers create module handlers.

* Use the [`substreams-ethereum` crate](https://crates.io/crates/substreams-ethereum) for Ethereum and other Ethereum-compatible chains.
* Use the [`substreams-solana` crate](https://crates.io/crates/substreams-solana) for the Solana blockchain.
* Use the [`substreams-antelope` crate](https://github.com/pinax-network/substreams-antelope) for the Antelope blockchain (by [Pinax Network](https://pinax.network/))

{% hint style="info" %}
**Note**: If a crate is not available for Substreams, you can use the `spkg` release for the chain, which includes the `Block` protobuf model, and generate the Rust structs yourself.
{% endhint %}

### Third-party libraries

Any third-party library capable of compiling `wasm32` can be used for execution in Substreams services.

Some libraries include kernel syscalls or other operations unavailable in the Substreams execution environment and cannot be compiled to WASM. The internal functionality of third-party libraries is an essential consideration for Substreams development.

Helpful information people found through the use of third-party libraries and Substreams together include:

* [`tiny_keccak`](https://docs.rs/tiny-keccak): an implementation of Keccak-derived functions specified in FIPS-202, SP800-185, and KangarooTwelve.
