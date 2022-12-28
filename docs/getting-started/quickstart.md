---
description: Get off the ground using Substreams by StreamingFast
---

# Quickstart

## Authentication

Get a StreamingFast API **Key** from: [https://app.streamingfast.io](https://app.streamingfast.io)

Get an API **Token** using:

{% code overflow="wrap" %}
```bash
export STREAMINGFAST_KEY=server_123123 # Make this your own
export SUBSTREAMS_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
```
{% endcode %}

{% hint style="info" %}
**Note**: See the [ authentication](../reference-and-specs/authentication.md) section for details.&#x20;
{% endhint %}

## Run your first Substreams

{% hint style="warning" %}
_**Important**: The Substreams CLI **must** be_ [_installed_](installing-the-cli.md) _to continue._
{% endhint %}

Once authenticated, run your first Substreams using:

{% code overflow="wrap" %}
```bash
$ substreams run -e mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg map_transfers --start-block 12292922 --stop-block +1
```
{% endcode %}

This [`run`](../reference-and-specs/command-line-interface.md#run) command starts a consumer, targeting the `--endpoint` serving [a given blockchain](../reference-and-specs/chains-and-endpoints.md), for the given [spkg package](../reference-and-specs/packages.md), starting at the given block, and stopping after processing one block. It will stream the output of the `map_transfers` [module](../developers-guide/modules/setting-up-handlers.md).

{% hint style="info" %}
If you prefer streaming using third-party languages, try the [Python](https://github.com/streamingfast/substreams-playground/tree/master/consumers/python) example.
{% endhint %}

## Platform independent Substreams

Substreams is platform independent, meaning you can use many different blockchains.&#x20;

Developers will typically target a specific blockchain and build a Substreams module targeted toward the chosen chain.&#x20;

Data is available for any blockchain that has a functional Firehose endpoint, either installed and [set up on-premise](https://firehose.streamingfast.io/firehose-setup/ethereum/installation-1), [provided by StreamingFast](../reference-and-specs/chains-and-endpoints.md) or other vendors.

{% hint style="info" %}
**Note**: The remaining documentation assumes the Substreams CLI and all other required dependencies have been installed and a [StreamingFast authentication token](../reference-and-specs/authentication.md) has been obtained.&#x20;
{% endhint %}

### **Basics**

The most basic approach to using Substreams is through the CLI, passing an endpoint and the name of the Rust function, or module handler, that the compute engine should process. The manifest defines the name of the module handler and the protocol buffer to use as the data definition for the Substreams module.&#x20;

The command demonstrates modules targeting the Ethereum blockchain and instructing the compute engine to execute the `map_basic_eth` Rust function.

#### **Basic Ethereum command line call**

{% code overflow="wrap" %}
```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams-ethereum-tutorial.yaml map_basic_eth --start-block 10000001 --stop-block +1
```
{% endcode %}

#### **Basic Solana command line call**

{% code overflow="wrap" %}
```bash
substreams run -e mainnet.sol.streamingfast.io:443 substreams-solana-tutorial.yaml map_basic_sol
```
{% endcode %}

Each blockchain has specific requirements for its data definition so the code in the module handlers will need to be updated accordingly.

### **Crates and packages**

Create a `Cargo.toml` at the root of your project:

```toml
[package]
name = "substreams-ethereum-tutorial"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
substreams = "0.5.0"
prost = "0.11"

[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```

Install the Rust crate for the desired chain. The crates have the protobuf models for the specific chain as well as helper code.

```bash
$ cargo add substreams-ethereum # or
$ cargo add substreams-solana 
```

{% hint style="success" %}
**Tip**: Crates should be used if they are available for the target blockchain.\
\
Alternatively, you can generate the Rust structs from one of the chain-specific `spkg` packages, which contain the protobuf modules. See [Rust crates](../reference-and-specs/rust-crates.md) for details.
{% endhint %}

## **Examples**

### Examples overview

The relationships for the flow of data are defined in the Substreams manifest. Further information is available in the documentation for [defining complex data strategies](../reference-and-specs/manifests.md) through manifest files.

### Ethereum example

Clone or download the Ethereum example codebase to get started. Find the example in the official GitHub repository.

[https://github.com/streamingfast/substreams-ethereum-tutorial](https://github.com/streamingfast/substreams-ethereum-tutorial)

Take a moment to explore the codebase. Note the chain name used through the different files including the [manifest](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/substreams-ethereum-tutorial.yaml), and the [TOML build configuration file](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/Cargo.toml).&#x20;

Also notice the module handler, defined in [lib.rs](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/src/lib.rs), and the custom protobuf definition in the proto directory named [basicexample.proto](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/proto/basicexample.proto).

Note that the module handler in the lib.rs file for the Ethereum example has code specific to the blockchain being targeted. The block structure for Ethereum blocks can be seen in the following code excerpt.

{% code title="src/lib.rs" overflow="wrap" %}
```rust
#[substreams::handlers::map]
fn map_basic_eth(block: ethpb::eth::v2::Block) -> Result<basicexample::BasicExampleProtoData, substreams::errors::Error> {
    log::info!("block.ver: {:#?}", block.ver);
    log::info!("block.number: {:#?}", block.number);
    Ok(basicexample::BasicExampleProtoData {version: block.ver})
}
```
{% endcode %}

The steps to follow for using the example include the following:&#x20;

* running the command to generate the required protobufs,
* using the Rust compiler to compile the example,&#x20;
* and sending commands to the Substreams CLI.

At the root of the example project, generate your structs from protobuf specified in the YAML:

{% code overflow="wrap" %}
```bash
substreams protogen substreams-ethereum-tutorial.yaml
```
{% endcode %}

Then compile using:

```shell
cargo build --release --target wasm32-unknown-unknown
```

You're ready to run:

{% code overflow="wrap" %}
```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams-ethereum-tutorial.yaml map_basic_eth --start-block 1000000 --stop-block +1
```
{% endcode %}

### Solana example

The procedure for using the Solana example is very similar to the Ethereum example.&#x20;

Clone or download the Solana example codebase. Find the example in the official GitHub repository.

[https://github.com/streamingfast/substreams-solana-tutorial](https://github.com/streamingfast/substreams-solana-tutorial)

After cloning the Solana example, take a moment to look through the repository. Key differences from the Ethereum example should begin to stand out immediately.

A very important difference between the two examples is the module handler code. Blockchains each have their own architectures, implementations, and data structures. Blocks for Ethereum or even Bitcoin will be constructed and designed differently. Some of the differences are small and subtle while others are not.

Notice the Solana module handler is accessing the `previous_blockhash` `blockhash` and `slot` fields of the block passed into the module handler by Substreams.&#x20;

The Ethereum example's module handler accessed the `ver` and `number`. These are differences in the block model for each of the blockchains.

{% code title="src/lib.rs" overflow="wrap" %}
```rust
#[substreams::handlers::map]
fn map_basic_sol(block: solpb::sol::v1::Block) -> Result<basicexample::BasicExampleProtoData, substreams::errors::Error> {
    log::info!("block.previous_blockhash: {:#?}", block.previous_blockhash);
    log::info!("block.blockhash: {:#?}", block.blockhash);
    log::info!("block.slot: {:#?}", block.slot);
    Ok(basicexample::BasicExampleProtoData {blockhash: block.blockhash})
}
```
{% endcode %}

Follow the same steps used for the Ethereum example for the Solana example.

When issuing the following command to run the Solana example take note of the different endpoint, map module name, and manifest filename.

{% code overflow="wrap" %}
```basic
substreams run -e mainnet.sol.streamingfast.io:443 substreams-solana-example.yaml sol_basic_mapper --start-block 10000000 --stop-block +1
```
{% endcode %}

## **Next steps**

The key takeaways at this point are:

1. Substreams is platform independent and can be used across many different blockchains.
2. Block data for each blockchain follows a different structure and model.
3. Each blockchain has a different endpoint.
4. Each blockchain has a different package.
5. Custom protobufs are created to pass data from one module to another.

{% hint style="info" %}
**Note**: Gaining a basic understanding of how Substreams works across multiple blockchains will enable developers to graduate to build even more complex solutions.&#x20;
{% endhint %}

Understanding map and store modules is the next step to understanding how to design and craft a fully directed acyclic graph in the Substreams manifest.

Additional information is available for understanding [modules](../concepts-and-fundamentals/modules.md), and sample code can be found in the [Developer's Guide](../developers-guide/overview.md).&#x20;

Visit the [Substreams Template](https://github.com/streamingfast/substreams-template) repository and [Substreams Playground](https://github.com/streamingfast/substreams-playground) to get up and running.

## **Troubleshooting and errors**

### **Requesting block types from the wrong chain endpoint**

{% code overflow="wrap" %}
```shell
Error: rpc error: code = InvalidArgument desc = validate request: input source "type:\"sf.solana.type.v1.Block\"" not supported, only "sf.ethereum.type.v2.Block" and 'sf.substreams.v1.Clock' are valid
```
{% endcode %}

A common mistake when first getting started is requesting data for one chain, such as Ethereum, and providing an incorrect chain endpoint, for a different blockchain. It's important to note that data from one chain is not compatible. The error is informing you about this issue.

To resolve the problem, double-check the code and settings within the Substreams codebase against the endpoint that's being sent to the Substreams CLI. The error is from an Ethereum codebase requesting Solana Blocks.&#x20;

Look through the codebase to see what is required by the blockchain configuration for your code. The blocks that are expected are different than what is being sent to the Substreams CLI.
