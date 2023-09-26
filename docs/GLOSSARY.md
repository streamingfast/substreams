
## CLI
`CLI`, which stands for command-line interface, is a text-based interface that allows you to input command to interact with a computer.
The [Substreams CLI](https://substreams.streamingfast.io/getting-started/installing-the-cli) allows you to deploy and manage your Substreams.

## Module
[Modules](https://substreams.streamingfast.io/developers-guide/modules) are small pieces of Rust code running in a WebAssembly (WASM) virtual machine. Modules have one or more inputs and an output.
For example, a module could receive an Ethereum block as input and emit a list of of transfer for that block as output.

There are two types of modules: `map` and `store`.

## map Module

`map` modules receive an input and emit an output (i.e. they perform a transformation).

## store Module

`store` modules write to key-value stores and are stateful. They are useful in combination with `map` modules to keep track of past data.

## Protocol Buffers (Protobuf)

[Protocol Buffers](https://protobuf.dev/) are a serializing format used to define module inputs and outputs in Substreams.
For example, a module might define a module called `map_tranfers` with an input object, `Transfer` (representing an Ethereum transaction), and an output object `MyTransfer` (representing a reduced version of an Ethereum transaction).

## Manifest
The [Substreams manifest](https://substreams.streamingfast.io/developers-guide/creating-your-manifest) (called `substreams.yml`) is a YAML file where you define all the configurations needed. For example, the modules of your Substreams (along with their intputs and outputs), or the Protobuf definitions used.

## WebAssembly (WASM)
[WebAssembly (WASM)](https://webassembly.org/) is a binary-code format used to run a Substreams. The Rust code used to define your Substreams transformations is packed into a WASM module, which you can use as an independent executable.

## Block
The `Block` Protobuf object contains all the blockchain information for a specific block number. EVM-compatible chains share the same [Block](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto) object, but non EVM-compatible chains must use [their corresponding Block Protobuf definition](https://substreams.streamingfast.io/reference-and-specs/chains-and-endpoints).

<figure><img src=".gitbook/assets/chains-endpoints.png" width="100%" /></figure>

## SPKG (.spkg)

[SPKG files]() contain Substreams definitions. You can create a `.spkg` file from a Substreams manifest using the `substreams pack` command. Then, you can use this file to share or run the Substreams independently.
The `.spkg` file contains everything needed to run a Substreams: Rust code, Protobuf definitions and the manifest.

## GUI

The CLI includes two commands to run a Substreams: `run` and `gui`. The `substreams run` command prints the output of the execution linearly for every block, while the `substreams gui` allows you to easily jump to the output of a specific block.

<figure><img src=".gitbook/assets/gui/gui.png" width="100%" /></figure>

## Sink

Substreams allows you to extract blockchain data and apply transformations to it. After that, you should choose **a place to send your transform data, which is called _sink_**.
A sink can be a [Postgres database](https://substreams.streamingfast.io/developers-guide/sink-targets/substreams-sink-postgres), [a file](https://substreams.streamingfast.io/developers-guide/sink-targets/substreams-sink-files) or a [custom solution of your choice](https://substreams.streamingfast.io/developers-guide/sink-targets/custom-sink-js).

## Parallel execution
[Parallel execution](https://substreams.streamingfast.io/developers-guide/parallel-execution) is the process of a Substreams module's code executing multiple segments of blockchain data simultaneously in a forward or backward direction.