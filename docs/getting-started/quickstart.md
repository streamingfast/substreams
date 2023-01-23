---
description: Get off the ground by using Substreams by StreamingFast
---

# Quickstart guide

## Authentication

Get a StreamingFast API **key** from: [https://app.streamingfast.io](https://app.streamingfast.io).

Get an API **token** by using:

{% code overflow="wrap" %}

```bash
export STREAMINGFAST_KEY=server_123123 # Use your own key
export SUBSTREAMS_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
```

{% endcode %}

Visit the[ authentication](../reference-and-specs/authentication.md) page for additional information.

## Run your first Substreams

{% hint style="success" %}
**Tip**: The [`substreams` CLI](../reference-and-specs/command-line-interface.md) [**must** **be installed** ](installing-the-cli.md)**to continue**.
{% endhint %}

After you have authenticated, you're ready to [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) your first Substreams by using:

{% code title="substreams run" overflow="wrap" %}

```bash
$ substreams run -e mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg map_transfers --start-block 12292922 --stop-block +1
```

{% endcode %}

The [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command starts a consumer by using the `--endpoint` serving [a given blockchain](../reference-and-specs/chains-and-endpoints.md), for the [spkg package](../reference-and-specs/packages.md). Processing starts at the given block, then stops after processing one block. The output of the `map_transfers` [module](../developers-guide/modules/setting-up-handlers.md) is streamed to the requesting client.

{% hint style="success" %}
**Tip**: Try the [Python](https://github.com/streamingfast/substreams-playground/tree/master/consumers/python) example if you prefer streaming by using third-party languages.
{% endhint %}

## Build a Substreams module

To create a simple "Substreams module" that extracts data from the Ethereum blockchain, you will need to use the [`substreams` CLI](https://substreams.streamingfast.io/reference-and-specs/command-line-interface) and obtain an authentication key.

Before continuing, ensure that your system [meets the basic requirements](https://substreams.streamingfast.io/developers-guide/installation-requirements) for Substreams development.

### Objectives

- Create your first Substreams module
- Use the [`substreams` CLI](https://substreams.streamingfast.io/reference-and-specs/command-line-interface) to run the module

### 1. Create Substreams manifest

To create a "Substreams module", you must first create the manifest file. This example manifest includes the minimal required fields to demonstrate the core values that you must provide.

To use the example manifest, copy and paste it into a new file named `substreams.yaml`. Save this file in the root directory of your Substreams module. You can find the example manifest in the [official GitHub repository for `substreams-ethereum-tutorial`](https://github.com/streamingfast/substreams-ethereum-tutorial).

```yaml
specVersion: v0.1.0
package:
  name: 'substreams_ethereum_tutorial'
  version: v0.1.0

protobuf:
  files:
    - basicexample.proto
  importPaths:
    - ./proto

imports:
  eth: https://github.com/streamingfast/sf-ethereum/releases/download/v0.10.2/ethereum-v0.10.4.spkg

binaries:
  default:
    type: wasm/rust-v1
    file: target/wasm32-unknown-unknown/release/substreams_ethereum_tutorial.wasm

modules:
  - name: map_basic_eth
    kind: map
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.basicexample.v1.BasicExampleProtoData
```

### 2. Create Rust manifest file

To complete your new Substreams module, you must also create a Rust manifest file.

To use the example Rust manifest file, copy and paste its content into a new file named [`Cargo.toml`](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/Cargo.toml). Save this file in the root directory of your Substreams module. It's important to provide a unique and useful value for the "name" field and to make sure that `crate-type = ["cdylib"]` is defined so the WASM is generated.

Also, include any dependencies on Substreams crates for helpers and `prost` for protobuf encoding and decoding.

Finally, use the values provided in the example for the `profile.release` section to build an optimized `.WASM` file for your module.

```toml
[package]
name = "substreams-ethereum-tutorial"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
substreams = "0.5.0"
substreams-ethereum = "0.8.0"
prost = "0.11"

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```

### 3. Create protobufs

Substreams modules are required to output protobuf encoded messages. The example protobuf definition from the [`substreams-ethereum-tutorial`](https://github.com/streamingfast/substreams-ethereum-tutorial) defines a field for the blockchain version. The value is set in the Substreams module handler.

Copy and paste the content for the example protobuf definition into a new file named [`BasicExampleProtoData.proto`](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/proto/basicexample.proto) and save it to a `proto` directory in the root, or main directory, of your Substreams module.

```
syntax = "proto3";

package eth.basicexample.v1;

message BasicExampleProtoData {
  int32 version = 1;
}
```

Use the `substreams protogen` command to generate the Rust code to communicate with the protobuf.

```bash
substreams protogen substreams-ethereum-tutorial.yaml
```

### 4. Create Substreams module handlers

Your Substreams module must contain a Rust library that houses the "module handlers." These handlers are responsible for handling blockchain data injected into the module at runtime.

To include this example module handler in your module, copy it into a new Rust source code file. Use the filename [`lib.rs`](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/src/lib.rs) for this file. Make sure to save the Rust library source code file in the `src` directory, located in the root directory of your Substreams module.

{% code overflow="wrap" %}

```rust
mod pb;

use pb::basicexample;

use substreams::{log, Hex};
use substreams_ethereum::{pb as ethpb};

#[substreams::handlers::map]
fn map_basic_eth(block: ethpb::eth::v2::Block) -> Result<basicexample::BasicExampleProtoData, substreams::errors::Error> {
    let header = block.header.as_ref().unwrap();
    log::info!("block.number: {:#?}", block.number);
    Ok(basicexample::BasicExampleProtoData {number: block.number, hash: Hex(&block.hash).to_string(), parent_hash: Hex(&header.parent_hash).to_string(), timestamp: header.timestamp.as_ref().unwrap().to_string()})
}
```

{% endcode %}

Compile your Substreams module.

```bash
cargo build --release --target wasm32-unknown-unknown
```

### 5. Execute

To execute, or run, the example use the `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command:

{% code overflow="wrap" %}

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml map_basic_eth --start-block 10000001 --stop-block +1
```

{% endcode %}

To ensure that the [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command is executed correctly, you need to pass the proper endpoint, manifest name, and module handler name. The `--start-block` and `--stop-block` flags are optional, but they can help limit the results that are returned to the client.

You have successfully created your first Substreams module that extracts data from the Ethereum blockchain.

## Next steps

- [Modules basics](https://substreams.streamingfast.io/concepts-and-fundamentals/modules)
- [Substreams fundamentals](https://substreams.streamingfast.io/concepts-and-fundamentals/fundamentals)
- [Protobuf schemas](https://substreams.streamingfast.io/developers-guide/creating-protobuf-schemas)
- [Substreams Template](https://github.com/streamingfast/substreams-template)
