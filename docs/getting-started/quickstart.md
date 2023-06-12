---
description: Get off the ground by using Substreams by StreamingFast
---

# Quickstart

## Run your first Substreams

You will first need to get a StreamingFast API **key** from [https://app.streamingfast.io](https://app.streamingfast.io). Using this API key, retrieve an API **token** by using:

{% code overflow="wrap" %}
```bash
export STREAMINGFAST_KEY=server_123123 # Use your own API key
export SUBSTREAMS_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
```
{% endcode %}

After you have authenticated, you're ready to [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) your first Substreams.

{% hint style="success" %}
**Tip**: The [`substreams` CLI](../reference-and-specs/command-line-interface.md) [**must** **be installed** ](installing-the-cli.md)**to continue**.
{% endhint %}

{% code title="substreams run" overflow="wrap" %}
```bash
$ substreams run -e mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-ethereum-quickstart/releases/download/1.0.0/substreams-ethereum-quickstart-v1.0.0.spkg map_block --start-block 12292922 --stop-block +1
```
{% endcode %}

The [`run`](../reference-and-specs/command-line-interface/#run) command starts a consumer by using the `--endpoint` serving [a given blockchain](../reference-and-specs/chains-and-endpoints.md), for the [spkg package](../reference-and-specs/packages.md). Processing starts at the given block, then stops after processing one block. The output of the `map_block` [module](../developers-guide/modules/setting-up-handlers.md) is streamed to the requesting client.

{% hint style="info" %}
**Note**: While Substreams technology is chain-agnostic, you must write your Substreams for a specific chain. In this quickstart, we are using Ethereum as our specific chain, general concepts given in the quick start applied to every Substreams supported chain.
{% endhint %}

## Build a Substreams module

In this section we are going to:

* Create your first Substreams module
* Use the [`substreams` CLI](../reference-and-specs/command-line-interface.md) to run the module

{% hint style="info" %}
**Note**: Before continuing, ensure that your system [meets the basic requirements](../developers-guide/installation-requirements.md) for Substreams development.
{% endhint %}

### Create Substreams manifest

To create a "Substreams module", you must first create the manifest file. This example manifest includes the minimal required fields to demonstrate the core values that you must provide.

To use the example manifest, copy and paste it into a new file named `substreams.yaml`. Save this file in the root directory of your Substreams module. You can find the example manifest in the [official GitHub repository for `substreams-ethereum-quickstart`](https://github.com/streamingfast/substreams-ethereum-quickstart).

```yaml
specVersion: v0.1.0
package:
  name: 'substreams_ethereum_quickstart'
  version: v1.0.0

protobuf:
  files:
    - block_meta.proto
  importPaths:
    - ./proto

binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/substreams.wasm

modules:
  - name: map_block
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:sf.ethereum.block_meta.v1.BlockMeta
```

### Create Rust manifest file

To complete your new Substreams module, you must also create a Rust manifest file.

To use the example Rust manifest file, copy and paste its content into a new file named [`Cargo.toml`](https://github.com/streamingfast/substreams-ethereum-quickstart/blob/main/Cargo.toml). Save this file in the root directory of your Substreams module. It's important to provide a unique and useful value for the "name" field and to make sure that `crate-type = ["cdylib"]` is defined so the WASM is generated.

Include any dependencies on Substreams crates for helpers and `prost` for protobuf encoding and decoding. Finally, use the values provided in the example for the `profile.release` section to build an optimized `.WASM` file for your module.

```toml
[package]
name = "substreams-ethereum-quickstart"
version = "1.0.0"
edition = "2021"

[lib]
name = "substreams"
crate-type = ["cdylib"]

[dependencies]
substreams = "0.5.0"
substreams-ethereum = "0.9.0"
prost = "0.11"

[profile.release]
lto = true
opt-level = 'z'
strip = "debuginfo"
```

{% hint style="success" %}
**Tip**: The `[profile.release]` section can dramatically reduce the size of your wasm code and its loading time in substreams engine. `lto = true` is always beneficial to performance. `opt-level = 'z'` optimizes for size, which results in a reduced "Time To First Byte", in some cases by several seconds, which is usually beneficial, but it may not always be the best choice for performance. [See more info](https://docs.rust-embedded.org/book/unsorted/speed-vs-size.html). We do, however, recommend it. The `strip = "debuginfo"` will remove useful information from stack traces, but the faster load time still make it a good choice when iterating in development.
{% endhint %}

### Create protobufs

Substreams modules are required to output protobuf encoded messages. The example protobuf definition from the [`substreams-ethereum-quickstart`](https://github.com/streamingfast/substreams-ethereum-quickstart) defines a simple `BlockMeta` message that contains that block's hash, number, parent hash and timestamp all in human readable form.

Copy and paste the content for the example protobuf definition into a new file named [`block_meta.proto`](https://github.com/streamingfast/substreams-ethereum-quickstart/blob/main/proto/block\_meta.proto) and save it to a `proto` directory in the root directory of your Substreams module.

```
syntax = "proto3";

package sf.ethereum.block_meta.v1;

message BlockMeta {
  string hash = 1;
  uint64 number = 2;
  string parent_hash = 3;
  string timestamp = 4;
}
```

Use the `substreams protogen` command to generate the Rust code to communicate with the protobuf.

```bash
substreams protogen substreams.yaml --exclude-paths="sf/substreams,google
```

{% hint style="info" %}
**Note**: The flag `--exclude-paths="sf/substreams,google` avoids generating files which are already provided implicitly.
{% endhint %}

The protobufs generate model must be referenced by a Rust module, to do so, create a file named `mod.rs` within the `src/pb` directory with the following content:

{% code overflow="wrap" %}
```rust
#[path = "sf.ethereum.block_meta.v1.rs"]
#[allow(dead_code)]
pub mod block_meta;
```
{% endcode %}

### Create Substreams module handlers

Your Substreams module must contain a Rust library that houses the module handlers, the code that is invoked to perform your customized logic. These handlers are responsible for handling blockchain data injected into the module at runtime, see [Substreams Modules](../developers-guide/modules/types.md) for further details about module and module handlers.

To include this example module handler in your module, copy it into a new Rust source code file named [`lib.rs`](https://github.com/streamingfast/substreams-ethereum-quickstart/blob/main/src/lib.rs) within the `src` directory.

{% code overflow="wrap" %}
```rust
mod pb;

use pb::block_meta::BlockMeta;
use substreams::Hex;
use substreams_ethereum::pb::eth;

#[substreams::handlers::map]
fn map_block(block: eth::v2::Block) -> Result<BlockMeta, substreams::errors::Error> {
    let header = block.header.as_ref().unwrap();

    Ok(BlockMeta {
        number: block.number,
        hash: Hex(&block.hash).to_string(),
        parent_hash: Hex(&header.parent_hash).to_string(),
        timestamp: header.timestamp.as_ref().unwrap().to_string(),
    })
}
```
{% endcode %}

Compile your Substreams module.

```bash
cargo build --release --target wasm32-unknown-unknown
```

{% hint style="info" %}
**Note**: If you have a lots of weird compilation errors like `cannot find macro 'assert' in this scope`, `cannot find tuple struct or tuple variant 'Some' in this scope`, you probably don't have the target `wasm32-unknown-unknown` installed in your Rust environment, see [Rust installation](../developers-guide/installation-requirements.md#wasm32-unknown-unknown-target) for instructions how to install it.
{% endhint %}

### Execute

To execute, or run, the example use the `substreams` [`run`](../reference-and-specs/command-line-interface.md#run) command:

{% code overflow="wrap" %}
```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml map_block --start-block 10000001 --stop-block +1
```
{% endcode %}

To ensure that the [`run`](../reference-and-specs/command-line-interface.md#run) command is executed correctly, you need to pass the proper endpoint, manifest name, and module handler name. The `--start-block` and `--stop-block` flags are optional, but they can help limit the results that are returned to the client.

You have successfully created your first Substreams module that extracts data from the Ethereum blockchain.

## Next steps

* [Modules basics](../concepts-and-fundamentals/modules.md)
* [Substreams fundamentals](../concepts-and-fundamentals/fundamentals.md)
* [Protobuf schemas](../developers-guide/creating-protobuf-schemas.md)
* [Substreams Template](https://github.com/streamingfast/substreams-template)
