
## CLI
`CLI`, which stands for command-line interface, is a text-based interface that allows you to input command to interact with a computer.
The [Substreams CLI](https://substreams.streamingfast.io/getting-started/installing-the-cli) allows you to deploy and manage your Substreams.

## Subgraph


## WebAssembly (WASM)
Binary-code format to run 

## Module
Modules are small pieces of Rust code running in a WebAssembly (WASM) virtual machine. Modules have one or more inputs and an output.
For example, a module could receive an Ethereum block as input and emit a list of of transfer for that block as output.

There are two types of modules: `map` and `store`.

## map Module

`map` modules receive an input and emit an output (i.e. they perform a transformation).

## store Module

`store` modules write to key-value stores and are stateful. They are useful in combination with `map` modules to keep track of past data.

## Protocol Buffers (Protobuf)

[Protocol Buffers](https://protobuf.dev/) are a serializing format used to define module inputs and outputs in Substreams.
For example, a module might define an input object, ``

## Block

The `Block` object contains all the blockchain information for a specific block number. Every chain 


## SPKG

## GUI

## Sink