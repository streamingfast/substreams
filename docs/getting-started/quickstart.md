---
description: Get off the ground by using Substreams by StreamingFast
---

# Quickstart

## Authentication

Get a StreamingFast API **key** from: [https://app.streamingfast.io](https://app.streamingfast.io).

Get an API **token** by using:

{% code overflow="wrap" %}

```bash
export STREAMINGFAST_KEY=server_123123 # Use your own key
export SUBSTREAMS_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
```

{% endcode %}

See the[ authentication](../reference-and-specs/authentication.md) page for details.&#x20;

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

## Build a Substreams from scratch

### 1. Create Substreams manifest

To create a "Substreams module" you need to first create the manifest file. The example manifest provided is taken from the `substreams-ethereum-tutorial`, available in [its official GitHub repository](https://github.com/streamingfast/substreams-ethereum-tutorial). The minimal amount of fields are defined in the example demonstrating the core values you need to provide.

Copy and paste the example manifest into a new file named substreams.yaml and save it to the root, or main, directory of your Substreams module.

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

### 2. Create Rust configuration file

Your new Substreams module also needs a Rust configuration file. The example configuration file is taken from the `substreams-ethereum-tutorial`.

Copy and paste the content for the example configuration file into a new file named `Cargo.toml` and save it to the root, or main, directory of your Substreams module.

It's important to provide a unique and useful value for the name field. Also, make sure the `crate-type = ["cdylib"]` is defined so WASM is generated.

Include any dependencies on Substreams crates for helpers and `prost` for protobuf encoding and decoding.

Lastly, use the values provided in the example for the `profile.release` section to build an optimized `.WASM` file for your module.

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
num-bigint = "0.4"

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```

### 3. Create protobufs

Substreams modules are required to output protobuf encoded messages. The example protobuf definition from the `substreams-ethereum-tutorial` defines a field for the blockchain version. The value is set in the Substreams module handler.

Copy and paste the content for the example protobuf definition into a new file named `BasicExampleProtoData.proto` and save it to a `proto` directory in the root, or main directory, of your Substreams module.

```
syntax = "proto3";

package eth.basicexample.v1;

message BasicExampleProtoData {
  int32 version = 1;
}
```

4. Create Rust library and module handlers

Every Substreams module contains a Rust library housing the "module handlers" responsible for handling blockchain data injected into them at runtime. The module handler in the EXAMPLE extracts the version from the Block object in the `block.ver` property. The value is logged to the terminal and assigned to the `version` field of the `BasicExampleProtoData` protobuf.

Copy the example module handler into a new Rust source code file and use the filename `lib.rs`. Make sure the Rust library source code file is saved to the `src` directory in the root, or main directory, of the Substreams module.

```rust
mod pb;

use pb::basicexample;

use substreams::{log};
use substreams_ethereum::{pb as ethpb};

#[substreams::handlers::map]
fn map_basic_eth(block: ethpb::eth::v2::Block) -> Result<basicexample::BasicExampleProtoData, substreams::errors::Error> {
    log::info!("block.ver: {:#?}", block.ver);
    log::info!("block.number: {:#?}", block.number);
    Ok(basicexample::BasicExampleProtoData {version: block.ver})
}
```

### 5. Execute

To execute, or run, the example use the `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command:

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams-ethereum-tutorial.yaml map_basic_eth --start-block 10000001 --stop-block +1
```

### 6. Next steps

Discuss the fact that substreams-template should be cloned when starting a new project.

Pointers to other materials.
