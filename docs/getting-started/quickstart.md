---
description: Get off the ground by using Substreams by StreamingFast
---

# Quickstart

## Authentication

Get a StreamingFast API **Key** from: [https://app.streamingfast.io](https://app.streamingfast.io).

Get an API **Token** by using:

{% code overflow="wrap" %}
```bash
export STREAMINGFAST_KEY=server_123123 # Use your own key
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

After you have authenticated, you're ready to run your first Substreams by using:

{% code overflow="wrap" %}
```bash
$ substreams run -e mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-template/releases/download/v0.2.0/substreams-template-v0.2.0.spkg map_transfers --start-block 12292922 --stop-block +1
```
{% endcode %}

The [`run`](../reference-and-specs/command-line-interface.md#run) command starts a consumer by using the `--endpoint` serving [a given blockchain](../reference-and-specs/chains-and-endpoints.md), for the [spkg package](../reference-and-specs/packages.md). Processing starts at the given block, then stops after processing one block. The output of the `map_transfers` [module](../developers-guide/modules/setting-up-handlers.md) is streamed to the requesting client.

{% hint style="info" %}
Try the [Python](https://github.com/streamingfast/substreams-playground/tree/master/consumers/python) example if you prefer streaming by using third-party languages
{% endhint %}

## Platform independent Substreams

Substreams is platform independent, meaning you can use many different blockchains.&#x20;

Developers select a specific blockchain and build a Substreams module tailored to the selected chain.&#x20;

Data is available for any blockchain exposing an operational Firehose endpoint, installed and set up in [an on-premises environment](https://firehose.streamingfast.io/firehose-setup/ethereum/installation-1), or [provided by StreamingFast](../reference-and-specs/chains-and-endpoints.md) or other vendors.

{% hint style="info" %}
**Note**: The remaining documentation assumes the Substreams CLI and all other required dependencies have been installed and a [StreamingFast authentication token](../reference-and-specs/authentication.md) has been obtained.&#x20;
{% endhint %}

### **Basics**

The most basic approach to use Substreams is through the CLI, passing an endpoint and the name of the Rust function, or module handler, used by the compute engine for processing. The manifest defines the name of the module handler and the protocol buffer to use as the data definition for the Substreams module.&#x20;

The commands demonstrate how Substreams modules work by using the selected blockchain and instructing the compute engine to execute the `map_basic_eth` or `map_basic_sol` Rust functions.

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

Different blockchains have specific requirements for their data definitions. The code in the module handlers needs to be updated to match the expectations of the different blockchains.

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

Install the Rust crate for the chain you want to use. The crates have the protobuf models for the specific chains and helper code.

```bash
$ cargo add substreams-ethereum # or
$ cargo add substreams-solana 
```

{% hint style="success" %}
**Tip**: Crates are used if they are available for the blockchain.\
\
Alternatively, generate the Rust structs from one of the chain-specific `spkg` packages, which contain the protobuf modules. See [Rust crates](../reference-and-specs/rust-crates.md) for details.
{% endhint %}

## **Examples**

### Examples overview

The relationships for the flow of data are defined in the Substreams manifest. Further information is available in the documentation for [defining complex data strategies](../reference-and-specs/manifests.md) through manifest files.

### Ethereum example

Clone or download the Ethereum example codebase to get started. Find the example in the official GitHub repository.

[https://github.com/streamingfast/substreams-ethereum-tutorial](https://github.com/streamingfast/substreams-ethereum-tutorial)

Take a moment to explore the codebase. Note the chain name used through the different files including the [manifest](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/substreams-ethereum-tutorial.yaml), and the [TOML build configuration file](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/Cargo.toml).&#x20;

Also notice the module handler, defined in [lib.rs](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/src/lib.rs), and the custom protobuf definition in the proto directory named [basicexample.proto](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/proto/basicexample.proto).

{% hint style="info" %}
**Note**: The module handler in the lib.rs file for the Ethereum example has code specific to the blockchain. The block structure for Ethereum blocks is viewable in the code excerpt.
{% endhint %}

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

The steps to follow to use the example include:&#x20;

* Running the command to generate the required protobufs
* Compiling the example by using the Rust compiler
* Sending commands to the Substreams CLI

Generate the structs from the protobuf specified in the YAML file by using:

{% code overflow="wrap" %}
```bash
substreams protogen substreams-ethereum-tutorial.yaml
```
{% endcode %}

Compile the project by using:

```shell
cargo build --release --target wasm32-unknown-unknown
```

Run the project by using:

{% code overflow="wrap" %}
```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams-ethereum-tutorial.yaml map_basic_eth --start-block 1000000 --stop-block +1
```
{% endcode %}

### Solana example

The procedure to use the Solana example is very similar to the Ethereum example.&#x20;

Clone or download the Solana example codebase. Find the example in the official GitHub repository.

[https://github.com/streamingfast/substreams-solana-tutorial](https://github.com/streamingfast/substreams-solana-tutorial)

After cloning the Solana example, take a moment to look through the repository. Differences from the Ethereum example stand out immediately.

A very important difference between the two examples is the module handler code. Different blockchains have their own architectures, implementations, and data structures. Blocks for Ethereum or even Bitcoin were constructed and designed differently. Some of the differences are small and subtle although others are not.

Notice the Solana module handler uses the `previous_blockhash`, `blockhash,` and `slot` fields of the block passed into the handler by Substreams. The Ethereum example's module handler uses the `ver` and `number` fields. The disparities in the field names are due to differences in the block model for the separate blockchains.

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

Take note of the different endpoint, map module name, and manifest filename in the command:

{% code overflow="wrap" %}
```basic
substreams run -e mainnet.sol.streamingfast.io:443 substreams-solana-example.yaml sol_basic_mapper --start-block 10000000 --stop-block +1
```
{% endcode %}

## **Next steps**

The takeaways are:

1. Substreams is platform independent and is used across many different blockchains.
2. Block data for individual blockchains follows a different structure and model.
3. Individual blockchains have different endpoints.
4. Individual blockchains have different packages.
5. Custom protobufs are created to pass data from one module to another.

{% hint style="info" %}
**Note**: Gaining a basic understanding of how Substreams works across multiple blockchains enables you to graduate to build even more complex solutions.
{% endhint %}

Understanding map and store modules are important for learning how to design and craft a fully directed acyclic graph in your Substreams manifest.

Additional information is available for understanding [modules](../concepts-and-fundamentals/modules.md) and sample code and projects are located in the [Developer's Guide](../developers-guide/overview.md).&#x20;

Visit the [Substreams Template](https://github.com/streamingfast/substreams-template) repository and [Substreams Playground](https://github.com/streamingfast/substreams-playground) to get up and running.

## **Troubleshooting and errors**

### **Requesting block types from the wrong chain endpoint**

{% code overflow="wrap" %}
```shell
Error: rpc error: code = InvalidArgument desc = validate request: input source "type:\"sf.solana.type.v1.Block\"" not supported, only "sf.ethereum.type.v2.Block" and 'sf.substreams.v1.Clock' are valid
```
{% endcode %}

A common mistake when first getting started is requesting data for one chain, such as Ethereum, and providing an incorrect chain endpoint, for a different blockchain. It's important to note, data from one chain is not compatible with the others. The RPC error is informing you about the Block data disparity issue.

To resolve the problem, double-check the code and settings within the Substreams codebase against the endpoint being sent to the Substreams CLI. The error is from an Ethereum codebase requesting Solana Blocks.&#x20;

Look through the codebase to see what is required by the blockchain configuration for your code. The blocks you're expecting are different than what is being sent to the Substreams CLI.
