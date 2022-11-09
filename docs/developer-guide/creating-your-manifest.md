---
description: StreamingFast Substreams manifest
---

# Manifest

## Overview

The manifest contains the details for the various aspects and components of a Substreams implementation.

Every Substreams implementation contains one manifest. The manifest is a YAML-based file and provides all of the key elements and definitions required for the implementation.

The manifest provides vital insights into the blockchain being targeted, the design of the data flow, the names and types of modules, and locations and names for protobuf definitions.

{% hint style="success" %}
**Tip**: Additional detailed information for [manifests](../reference-and-specs/manifests.md) is available in the Substreams reference section.
{% endhint %}

## Example Manifest

The manifest below is from the [Substreams Template example](https://github.com/streamingfast/substreams-template) accompanying the developer guide.

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

## Manifest Walkthrough

### `imports.eth`&#x20;

Substreams consumes blocks and depends on a Substreams [package](../reference-and-specs/packages.md) matching the target blockchain.&#x20;

{% hint style="info" %}
**Note**: The Substreams Template references a package specific to the Ethereum blockchain referenced in the manifest as `ethereum-v0.10.4.spkg`.
{% endhint %}

### `protobuf.files`

The `protobuf.files` contains a list of protobuf files for the current Substreams implementation.&#x20;

{% hint style="info" %}
**Note**: The Substreams Template references the Ethereum-specific `erc721.proto` protobuf.&#x20;
{% endhint %}

### `protobuf.importPaths`

The `protobuf.importPaths` contains the paths to the protobufs for the current Substreams implementation.

## Module Definitions

The manifest defines a list of [modules](../concepts/modules.md) used in the Substreams implementation.&#x20;

The modules are Rust functions containing the business logic for the implementation.

{% hint style="info" %}
**Note**: The manifest in the Substreams Template example lists two modules: `block_to_transfers` and `nft_state.`
{% endhint %}

### **`block_to_transfers`**

The `block_to_transfers` module extracts all ERC721 transfers related to a specific smart contract address. The module receives Ethereum blocks as [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto).

The output for the `blocks_to_transfers` module is a list of ERC721 transfers. The business logic for `block_to_transfers` module is written as a Rust function.

{% hint style="info" %}
**Note**: The `initialBlock` is set to `12287507` in the Substreams Template example because the first transfers of tokens originated from the contracts at that block.
{% endhint %}

### **`nft_state`**

The `nft_state` store module receives transfers in each block extracted by the mapper. The store is a `count` of ERC721 tokens for a holder.&#x20;

The inputs of the module are protobuf models defined as: `proto:eth.erc721.v1.Transfers`.&#x20;

The `eth.erc721.v1.Transfers` protobuf module represents a list of ERC721 transfers in a  block.&#x20;

{% hint style="info" %}
**Note**: The `eth.erc721.v1.Transfers` protobuf module is also used as the output for the `map` module.
{% endhint %}

The stores `valueType` is `int64` and the merge strategy is `add.`
