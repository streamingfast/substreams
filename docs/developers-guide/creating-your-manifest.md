---
description: StreamingFast Substreams manifest
---

# Manifest

## Manifest Overview

The manifest contains the details for the various aspects and components of a Substreams module.

Every Substreams module contains one manifest. The manifest is a YAML-based file and provides vital insights into the blockchain being targeted, the design of the data flow, the names and types of modules, and locations and names for protobuf definitions.

{% hint style="success" %}
**Tip**: Additional [detailed information for manifests](../reference-and-specs/manifests.md) is available in the Substreams reference section.
{% endhint %}

## Example manifest

The manifest from the [Substreams Template example](https://github.com/streamingfast/substreams-template) is used in the Substreams documentation.

{% hint style="info" %}
**Note**: Learn more about Substreams manifests and different blockchains through the [list of maintained Substreams examples](reference-and-specs/examples.md) provided by StreamingFast

* The [example manifest](https://github.com/streamingfast/substreams-template/blob/develop/substreams.yaml) in the Substreams documentation is specific to the Ethereum blockchain.
{% endhint %}

{% code title="substreams.yaml" overflow="wrap" lineNumbers="true" %}
```yaml
specVersion: v0.1.0
package:
  name: 'substreams_example'
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
  - name: map_transfers
    kind: map
    initialBlock: 12287507
    inputs:
      - source: sf.ethereum.type.v2.Block
    output:
      type: proto:eth.erc721.v1.Transfers

  - name: store_transfers
    kind: store
    initialBlock: 12287507
    updatePolicy: add
    valueType: int64
    inputs:
      - map: map_transfers
```
{% endcode %}

View the [`substreams.yaml`](https://github.com/streamingfast/substreams-template/blob/develop/substreams.yaml) file in the repository.

## Manifest walkthrough

{% hint style="success" %}
**Tip**: When writing and checking your `substreams.yaml` file, it may help to check your manifest against our [JSON schema](https://json-schema.org/) to ensure there are no problems. JSON schemas can be used in [Jetbrains](https://www.jetbrains.com/help/idea/json.html#ws_json_schema_add_custom) and [VSCode](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml). Our manifest schema can be seen [here](../../schemas/manifest-schema.json).
{% endhint %}

### `imports.eth`

Substreams consumes blocks and depends on a Substreams [package](../reference-and-specs/packages.md) matching the target blockchain. The package is referenced by `imports.`

{% hint style="info" %}
**Note**: The Substreams Template references a package specific to the Ethereum blockchain, referenced in the manifest as `ethereum-v0.10.4.spkg`. The Solana SPL Token Transfers manifest references `solana-v0.1.0.spkg`.
{% endhint %}

### `protobuf.files`

The `protobuf.files` contains a list of protobuf files for the current Substreams module.

{% hint style="info" %}
**Note**: The Substreams Template references the Ethereum-specific `erc721.proto` protobuf while the Solana SPL Token Transfers example references the Solana-specific `solana_spl.proto`.
{% endhint %}

### `protobuf.importPaths`

The `protobuf.importPaths` contains the paths to the protobufs for the current Substreams module.

## Module definitions

The manifest defines a list of [modules](../concepts-and-fundamentals/modules.md) used in the Substreams module.

The modules are Rust functions containing the business logic for the module.

{% hint style="info" %}
**Note**: The manifest in the Substreams Template example lists two modules: `map_transfers` and `store_transfers.` The official naming convention for Substreams modules prefixes the module name by using `map_` or `store_` depending on the type.
{% endhint %}

### **`map_transfers`**

The `map_transfers` module extracts all ERC721 transfers related to a specific smart contract address. The module receives Ethereum blocks as [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto).

The output for the `map_transfers` module is a list of ERC721 transfers. The business logic for `map_transfers` module is written as a Rust function.

{% hint style="info" %}
**Note**: The `initialBlock` is set to `12287507` in the Substreams Template example because the first transfers of tokens originated from the contracts at that block.
{% endhint %}

### **`store_transfers`**

The `store_transfers` store module receives transfers in the blocks extracted by the mapper. The store is a `count` of ERC721 tokens for a holder.

The inputs of the module are protobuf models defined as: `proto:eth.erc721.v1.Transfers`.

The `eth.erc721.v1.Transfers` protobuf module represents a list of ERC721 transfers in a block.

{% hint style="info" %}
**Note**: The `eth.erc721.v1.Transfers` protobuf module is also used as the output for the `map` module.
{% endhint %}

The store's `valueType` is `int64` and the merge strategy is `add.`
