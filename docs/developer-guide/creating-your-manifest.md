---
description: StreamingFast Substreams manifest
---

# Manifest

## Overview

The Substreams manifest is the center of control for a Substreams implementation.&#x20;

Every Substreams implementation contains one manifest. The manifest is a YAML-based file and provides all of the key elements and definitions required.

The manifest provides vital insights into the blockchain being targeted, the design of the data flow, and the names and types of modules and protobufs.

{% hint style="success" %}
**Tip**: Additional information for [manifests](../reference-and-specs/manifests.md) is available in the Substreams reference section.
{% endhint %}

## Substreams Modules

The manifest defines a list of [modules](../concepts/modules.md) used in the Substreams implementation.&#x20;

The manifest will link to the Rust code that implements the business logic for the modules&#x20;

The example manifest below shows the fields and values required for a Substreams implementation.

{% hint style="info" %}
**Note**: The example below contains Ethereum-specific entries, such as [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto).&#x20;
{% endhint %}

{% hint style="success" %}
**Tip**: Substreams developers working with other blockchains will use values and objects specific to the chain they're targeting, such as [`sf.solana.type.v1.Block`](https://github.com/streamingfast/firehose-solana/blob/develop/proto/sf/solana/type/v2/type.proto) seen in the [Solana SPL Token Transfers Substreams](https://github.com/streamingfast/substreams-playground/tree/master/modules/sol-spl-tokens) example.
{% endhint %}

{% code title="substreams.yaml" overflow="wrap" lineNumbers="true" %}
```yaml
specVersion: v0.1.0
package:
  name: "substreams_example"
  version: v0.1.0

imports:
  eth: https://github.com/streamingfast/sf-ethereum/releases/download/v0.10.2/ethereum-v0.10.4.spkg

protobuf:
  files:
    - erc721.proto
  importPaths:
    - ./proto

binaries:
  default:
    type: wasm/rust-v1
    file: ./target/wasm32-unknown-unknown/release/substreams_example.wasm

modules:
  - name: block_to_transfers
    kind: map
    initialBlock: 12287507
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.erc721.v1.Transfers

  - name: nft_state
    kind: store
    initialBlock: 12287507
    updatePolicy: add
    valueType: int64
    inputs:
      - map: block_to_transfers

```
{% endcode %}

View this file in the repo by visiting the following link.

[https://github.com/streamingfast/substreams-template/blob/develop/substreams.yaml](https://github.com/streamingfast/substreams-template/blob/develop/substreams.yaml)

## Manifest Details

### `imports.eth`&#x20;

Substreams consumes blocks and depends on a Substreams [package](../reference-and-specs/packages.md) matching the target blockchain.&#x20;

{% hint style="info" %}
**Note**: The Substreams Template example contains references specific to the Ethereum blockchain.
{% endhint %}

### `protobuf.files`

The `protobuf.files` contains a list of protobuf files for the current Substreams implementation.&#x20;

{% hint style="info" %}
**Note**: The Substreams Template references Ethereum-specific protobufs.&#x20;
{% endhint %}

### `protobuf.importPaths`

The `protobuf.importPaths` contains the paths to the protobufs for the current Substreams implementation.

{% hint style="info" %}
**Note**: The example Substreams Template manifest lists two modules: `block_to_transfers` and `nft_state.`
{% endhint %}

### **`block_to_transfers`**

The `block_to_transfers` map module in the example Substreams Template receives an Ethereum block and extracts all ERC721 transfers related to a smart contract address into an object.&#x20;

{% hint style="info" %}
**Note**: The module receives Ethereum blocks with a [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto)protobuf definition. Block definitions are _chain specific_ and _must be versioned_.
{% endhint %}

#### Module Outputs

The outputs of the module are protobuf models defined as `proto:eth.erc721.v1.Transfers` representing a list of ERC721 transfers within an Ethereum block.

The module is linked to the Web Assembly (WASM) code containing the business logic.&#x20;

The Rust function implementing the business logic for the module is defined by the `block_to_transfers` module.

The first transfers of tokens originated from the contracts at block `12287507`. For this reason, `initialBlock` is used in the `map` module.

### **`nft_state`**

The `nft_state` `store` module takes the transfers per block, extracted in the mapper, as input and keeps track of the token count for a given holder.&#x20;

Inputs of the module are a custom protobuf model defined as `proto:eth.erc721.v1.Transfers`.&#x20;

The `eth.erc721.v1.Transfers` protobuf module represents a list of ERC721 transfers in a given block, and is used as the output for the `map` module defined above.

The given store will stores a `count` of ERC721 tokens for each holder. The store `valueType` is `int64` and the merge strategy is set to `add.`
