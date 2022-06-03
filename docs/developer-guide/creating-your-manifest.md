# Creating your Manifest

Firstly, create and step into your directory:

```bash
mkdir substreams_example
cd substreams_example
```

A Substreams manifest mainly defines a list of [modules](../concepts/modules.md). A module definition will generally contain a kind, either [`map`](../concepts/modules.md#a-map-module) or [`store`](../concepts/modules.md#a-store-module). It will also have a link to the Rust code that implements the business logic of the module, we call this the `module handler`. The `module handler` is a list of `inputs` for the modules, and a list of `outputs`.

For our manifest we will use:

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
      - source: sf.ethereum.type.v1.Block
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

Let's review a few important entries:

* `imports.eth` : Our `Substreams` will consume Ethereum blocks, thus we will depend on the Ethereum Substreams package. You can find out more about `Substreams` packages [here](../reference-and-specs/packages.md).
* `protobuf.files`: The list of our `Substreams` custom `Protobuf` files. We will create these files in the following step.
* `protobuf.importPaths`: The locations of our custom `Protobuf` files.

Furthermore, the manifest lists two modules: `block_to_transfers` and `nft_state`, where the former is a module of kind `map` and the latter is a module of kind `store`.

**`block_to_transfers`**

The `block_to_transfers` map module will take an Ethereum block as an input and will extract all ERC721 Transfers related to our contract into an object. The inputs of the module are:

* An Ethereum block, with the `Protobuf` definition of [`sf.ethereum.type.v1.Block`](https://github.com/streamingfast/sf-ethereum/blob/develop/proto/sf/ethereum/type/v1/type.proto). This Ethereum block definition is one we will provide. The block definition is chain specific and must be versioned, so if you are building a Substreams on NEAR you will use the StreamingFast NEAR block definition.

The outputs of the module are:

* A custom `Protobuf` model that we will define as `proto:eth.erc721.v1.Transfers`. This `Protobuf` module represent a list of ERC721 transfers in a given block.

Furthermore, we link the module to the wasm code (Rust code compiled as web assembly) containing the business logic. The Rust function which implements the modules' business logic is defined by the module name and is called `block_to_transfers` in this example.

Lastly, since we know that the first transfers of tokens originating from the contracts occurs at block `12287507,` we specify an `initialBlock` in our `map` module.

**`nft_state`**

The `nft_state` `store` module will take as input the transfers per block that we have extracted in the mapper, and keep track of the token count for a given holder. The inputs of the module are:

* A custom `Protobuf` model that we will define as `proto:eth.erc721.v1.Transfers`. This `Protobuf` module represent the list of ERC721 transfers in a given block. It is the output for the `map` module defined above.

The given store will simply store a `count` of ERC721 tokens per holder, thus our store `valueType` is `int64`. Lastly our merge strategy is `add.`
