---
description: StreamingFast Substreams manifest creation
---

# Manifest Creation

### Manifest Overview

A Substreams manifest primarily defines a list of [modules](../concepts/modules.md). Module definitions will generally contain a kind of either `map` or `store`.&#x20;

The manifest will link to the Rust code that implements the business logic of the module, also known as the `module handler`. The `module handler` is a list of `inputs` for the modules, and a list of `outputs`.

### Manifest YAML Creation

The example Substreams manifest provided below shows the fields and values that need to be present in the YAML configuration file.

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

### Manifest in Detail

#### `imports.eth`&#x20;

Substreams consumes Ethereum blocks and depends on the Ethereum Substreams package. _Note,_ _learn more about_ [_packages_](../reference-and-specs/packages.md) _in the reference and specs section of the documentation._

#### `protobuf.files`

The list of Substreams custom Protobuf files.&#x20;

#### `protobuf.importPaths`

The locations of custom Protobuf files.

The manifest lists two modules: `block_to_transfers` and `nft_state`, where the former is a module of kind `map` and the latter is a module of kind `store`.

**`block_to_transfers`**

The `block_to_transfers` map module will take an Ethereum block as an input and will extract all ERC721 Transfers related to our contract into an object. The inputs of the module are Ethereum blocks with the Protobuf definition of [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto).&#x20;

This Ethereum block definition is one provided by  StreamingFast. The block definition is chain specific and must be versioned. Substreams on NEAR will use the StreamingFast NEAR block definition.

The outputs of the module are custom `Protobuf` models  defined as `proto:eth.erc721.v1.Transfers`. This `Protobuf` module represents a list of ERC721 transfers in any given block.

The module is linked to the wasm code containing the business logic. The modules are Rust code compiled as web assembly.&#x20;

The Rust function which implements the modules business logic for the module is defined by the module name and is called `block_to_transfers` (in this example).

The first transfers of tokens originated from the contracts occurred at block `12287507,` so `initialBlock` is used in the `map` module.

**`nft_state`**

The `nft_state` `store` module will take the transfers per block that we have extracted in the mapper as input and keep track of the token count for a given holder.&#x20;

Inputs of the module are:

* A custom `Protobuf` model that we will define as `proto:eth.erc721.v1.Transfers`. This `Protobuf` module represent the list of ERC721 transfers in a given block. It is the output for the `map` module defined above.

The given store will simply store a `count` of ERC721 tokens per holder, thus our store `valueType` is `int64`. Lastly our merge strategy is `add.`
