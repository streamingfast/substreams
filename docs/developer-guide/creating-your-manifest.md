---
description: StreamingFast Substreams manifest creation
---

# Manifest Creation

### Manifest Overview

The Substreams manifest provides all of the key elements for the implementation. One manifest is required for each Substreams implementation.&#x20;

The Substreams manifest outlines the implementation and provides vital insights into the blockchain being targeted, the design of the data flow, and the names and types of modules and module handlers.

{% hint style="info" %}
**Tip**: Additional information for [manifests](../reference-and-specs/manifests.md) is available in the Substreams reference section.
{% endhint %}

#### Substreams Modules

A Substreams manifest defines a list of [modules](../concepts/modules.md). Module definitions themselves contain a `kind` that is set to either `map` or `store`.&#x20;

The manifest will link to the Rust code that implements the business logic of the module, also known as a `module handler`. The `module handler` manifest entry a list of `inputs` and a list of `outputs` for each module.

### Manifest YAML Creation

The example manifest below shows the fields and values required in the YAML manifest configuration file for a Substreams implementation.

{% hint style="info" %}
**Note**: The example below contains Ethereum-specific entries, such as `sf.ethereum.type.v2.Block`.&#x20;



Developers working with other blockchains will use values and objects specific to the chain they're targeting, such as `sf.solana.type.v1.Block` for Solana.
{% endhint %}

{% code title="substreams.yaml" %}
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

### Manifest in Detail

#### `imports.eth`&#x20;

Substreams consumes blocks and depends on a Substreams package matching the target blockchain. The Substreams Template example contains references specific to the Ethereum blocks.  &#x20;

{% hint style="info" %}
**Note**_:_ Learn more about [packages](../reference-and-specs/packages.md) in the reference and specs section of the documentation.
{% endhint %}

#### `protobuf.files`

The `protobuf.files` contains a list of Substreams custom protobuf files for the current implementation. The Substreams Template contains references to Ethereum-specific protobuf definitions and files.&#x20;

#### `protobuf.importPaths`

The `protobuf.importPaths` conatins the locations of the custom protobuf files for the current implementation.

The example Substreams Template manifest lists two modules: `block_to_transfers` and `nft_state.`&#x20;

The former is a module of kind `map` and the latter is a module of kind `store`.

**`block_to_transfers`**

The `block_to_transfers` map module will take an Ethereum block as an input and will extract all ERC721 Transfers related to the contract into an object.&#x20;

The inputs of the module are Ethereum blocks with the protobuf definition of [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto).

{% hint style="warning" %}
**Important**: Block definitions are _chain specific_ and _must be versioned_.
{% endhint %}

The outputs of the module are custom protobuf models defined as `proto:eth.erc721.v1.Transfers` representing a list of ERC721 transfers contained within any given block.

The module is linked to the Web Assembly (WASM) code containing the business logic. The modules are written in the Rust programming language and compiled to WASM.&#x20;

The Rust function implementing the business logic for the module is defined by the `block_to_transfers` module.

The first transfers of tokens originated from the contracts at block `12287507`. For this reason, `initialBlock` is used in the `map` module.

**`nft_state`**

The `nft_state` `store` module takes the transfers per block, extracted in the mapper, as input and keeps track of the token count for a given holder.&#x20;

Inputs of the module are a custom protobuf model defined as `proto:eth.erc721.v1.Transfers`.&#x20;

The `eth.erc721.v1.Transfers` protobuf module represents a list of ERC721 transfers in a given block, and is used as the output for the `map` module defined above.

The given store will stores a `count` of ERC721 tokens for each holder. The store `valueType` is `int64` and the merge strategy is set to `add.`
