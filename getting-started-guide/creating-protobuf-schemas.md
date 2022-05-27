# Creating Protobuf Schemas

Now that our manifest has been written, it is time to create your custom _Protobuf_ definition, the one we'll use as an input/output in your manifest file.&#x20;

Protocol Buffers are Google's language-neutral extensible mechanism for serializing structured data – think XML, but smaller, faster, and simpler. If you have not used _Protobuf_ before, here are a couple of resources to get started:

* Protobuf - [https://developers.google.com/protocol-buffers](https://developers.google.com/protocol-buffers)
* Tutorials - [https://developers.google.com/protocol-buffers/docs/tutorials](https://developers.google.com/protocol-buffers/docs/tutorials)

We have defined a _protobuf model_ as `proto:eth.erc721.v1.Transfers` which represents a list of ERC721 transfers.

Firstly, let's create the `proto` folder:

```bash
mkdir proto
cd proto
```

and in there, create our first protobuf definition file:

{% code title="erc721.proto" %}
```protobuf
syntax = "proto3";

package eth.erc721.v1;

message Transfers {
  repeated Transfer transfers = 1;
}

message Transfer {
  bytes from = 1;
  bytes to = 2;
  uint64 token_id = 3;
  bytes trx_hash = 4;
  uint64 ordinal = 5;
}
```
{% endcode %}

Now, we need to setup some `rust` code to generate our `rust` files from the `Protobuf` defined above.

We will first initiate a `rust` project in our Substream directory.

```bash
cargo init
# rename ./src/main.rs to ./src/lib.rs
mv ./src/main.rs ./src/lib.rs
```

Now, lets edit the newly created `Cargo.toml` file to look like this:

{% code title="Cargo.toml" %}
```rust
[package]
name = "substreams-example"
version = "0.1.0"
description = "Substream template demo project"
edition = "2021"
repository = "https://github.com/streamingfast/substreams-template"

[lib]
crate-type = ["cdylib"]

[dependencies]
substreams= { git = "https://github.com/streamingfast/substreams", branch="develop" }

[build-dependencies]
prost-build = "0.10.1"

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```
{% endcode %}

Let's go through the important changes. Our `rust` code will be compiled in [`wasm`](https://webassembly.org/). Think of `wasm` code as a binary instruction format that can be run in a virutal machine. When your `rust` code is compiled it will generate a `.so` file.&#x20;

**Let's break down the file**

Since we are building a `rust` a dynamic system library, after the `package`, we first need to specify:

```rust
...

[lib]
crate-type = ["cdylib"]
```

We then need to specify our `dependencies`. We will be using the Substream crate in our handlers, and we will be using the `prost-build` crate during our build step to generate the `rust` files from our `.proto`.&#x20;

We will then need to create a `build.rs` file that will use the `prost-build` crate to generate our `rust` file from our `Protobuf` definition.

```rust
use std::io::Result;
fn main() -> Result<()> {
    let mut prost_build = prost_build::Config::new();
    prost_build.out_dir("./src/pb");
    prost_build.compile_protos(&["erc721.proto"], &["./proto"])
}
```

the `build.rs`  configures a few paths that are used by `prost_build`.

* `../src/pb` is the destination folder for the `rust` generated `Protobuf` files.
* `erc721.proto` & `proto` acts as the source of our `Protobuf` definition.

Lastly we will need to run our `build` command to generate the `rust` `Protobuf` files.

```rust
cargo build --target wasm32-unknown-unknown --release
```

