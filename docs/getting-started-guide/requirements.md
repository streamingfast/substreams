# Requirements

#### Install Rust

Before we start creating any substream we will need to setup our development environment. Substreams are written in the [Rust programming language](https://www.rust-lang.org/)

There are [several ways to install Rust](https://www.rust-lang.org/tools/install), but for the sake of brevity:

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env # to configure your current shell
```

#### Install Protocol Buffer compiler

You will also need to install protocol buffer compiler. Again, there are multiple ways on how to do it. Here is the official documentation of [protocol buffer compiler](https://grpc.io/docs/protoc-installation/).

{% hint style="info" %}
If you forget to install `protoc`, when generating the definitions, you might see error about `cmake` not defined, this is a fallback when `protoc` is not found.
{% endhint %}

**Install Substream CLI tool**

You can run through the [Getting Started](../../#getting-started) to install the **`substream`** CLI Tool
