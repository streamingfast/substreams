# Installation

### Install Rust

Before we start creating any Substream we will need to setup our development environment. Substreams are written in the [Rust programming language](https://www.rust-lang.org/)

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of brevity, this is the easiest:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

### Install `protoc`

`protoc` is the reference Protocol Buffer compiler. It is needed to generate code for Rust and other languages, out of the protobuf definitions you will create or get through third-party Substreams packages.

Here is the official documentation of [protocol buffer compiler](https://grpc.io/docs/protoc-installation/).

{% hint style="info" %}
If you forget to install `protoc`, when generating the definitions, you might see error about `cmake` not defined, this is a fallback when `protoc` is not found.
{% endhint %}

### Install `protoc-gen-prost`

This tool helps you render Rust structures out of protobuf definitions, for using in your Substreams modules. It is called by `protoc` following their plugin system.

Install it with:

```bash
cargo install protoc-gen-prost
```

Read more about it here: [https://crates.io/crates/protoc-gen-prost-crate](https://crates.io/crates/protoc-gen-prost-crate)

### Install `buf`

[https://buf.build](https://buf.build) is a tool used to simplify the generation of typed structures in any language. It invokes `protoc` and simplifies a good number of things. Substreams Packages are compatible with [buf Images](https://docs.buf.build/reference/images).

See the [installation instructions here](https://docs.buf.build/installation).

### Install **** the `substreams` CLI tool

You can run through the [Getting Started](broken-reference) to install the Substreams CLI Tool
