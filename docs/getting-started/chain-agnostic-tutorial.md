---
description: StreamingFast Substreams chain-agnostic tutorial and examples
---

# Chain-agnostic Tutorial

## Chain-agnostic Substreams

Substreams is chain agnostic, meaning developers can work with many different blockchains.&#x20;

Developers will typically target a specific blockchain and build a Substreams module targeted toward the chosen chain.&#x20;

Data is available for any blockchain with a functional Firehose endpoint, either installed and [set up on-premise](https://firehose.streamingfast.io/firehose-setup/ethereum/installation-1), [provided by StreamingFast](../reference-and-specs/chains-and-endpoints.md) or other vendors.

{% hint style="info" %}
**Note**: The remaining documentation assumes the Substreams CLI has been installed along with all other required dependencies, and a [StreamingFast authentication token](../reference-and-specs/authentication.md) has been obtained.&#x20;
{% endhint %}

Reading through the Substreams [fundamentals](../concept-and-fundamentals/fundamentals.md) is also suggested to understand how all of the different technologies work together.

### **Basics**

The most basic approach to working with Substreams is through the CLI, passing an endpoint and the name of the Rust function, or module handler, that the compute engine should process. The manifest defines the name of the module handler and the protocol buffer to use as the data definition for the Substreams module.&#x20;

The command shown below demonstrates modules targeting the Ethereum blockchain and instructing the compute engine to execute the `map_basic_eth` Rust function.

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

### **Crates & Packages**

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

Install the create for the chain you are working with. These contain the protobuf models for the specific chain as well as helper code around them.

```bash
$ cargo add substreams-ethereum # or
$ cargo add substreams-solana 
```

{% hint style="success" %}
**Tip**: Crates should be used if they are available for the target blockchain.
{% endhint %}

When a crate is not available you should reference a package.&#x20;

* Packages contain protobuf definitions only.&#x20;
* Packages do not contain any generated Rust code.&#x20;
* Packages are not required in the Substreams manifest.

You can use the packages provided by StreamingFast for their development initiatives.

**Package for the Ethereum blockchain**

[https://github.com/streamingfast/sf-ethereum/releases/download/v0.10.2/ethereum-v0.10.4.spkg](https://github.com/streamingfast/sf-ethereum/releases/download/v0.10.2/ethereum-v0.10.4.spkg)

**Package for the Solana blockchain**

[https://github.com/streamingfast/sf-solana/releases/download/v0.1.0/solana-v0.1.0.spkg](https://github.com/streamingfast/sf-solana/releases/download/v0.1.0/solana-v0.1.0.spkg)

{% hint style="info" %}
**Note**: Fully developed Substreams modules can be packaged and used when creating new Substreams modules.&#x20;
{% endhint %}

Additional information on [Substreams packages](../reference-and-specs/packages.md) is available in the documentation.

## **Examples**

### Examples Overview

The relationships for the flow of data are defined in the Substreams manifest. Further information is available in the documentation for [defining complex data strategies](../reference-and-specs/manifests.md) through manifest files.

### Ethereum Example

Clone or download the Ethereum example codebase to get started. Find the example in the official GitHub repository.

[https://github.com/streamingfast/substreams-ethereum-tutorial](https://github.com/streamingfast/substreams-ethereum-tutorial)

After the Git repo has been cloned, take a moment to explore the codebase. Note the chain name used through the different files including the [manifest](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/substreams-ethereum-tutorial.yaml), and the [TOML build configuration file](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/Cargo.toml).&#x20;

Also notice the module handler, defined in [lib.rs](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/src/lib.rs), and the custom protobuf definition in the proto directory named [basicexample.proto](https://github.com/streamingfast/substreams-ethereum-tutorial/blob/main/proto/basicexample.proto).

Note that the module handler in the lib.rs file for the Ethereum example has code specific to the blockchain being targeted. The block structure for Ethereum blocks can be seen in the following code excerpt.

{% code title="src/lib.rs" overflow="wrap" %}
```rust
#[substreams::handlers::map]
fn map_basic_eth(block: ethpb::eth::v2::Block) -> Result<basicexample::BasicExampleProtoData, substreams::errors::Error> {
    // Extract data from the Ethereum Block and log to the console.
    // The data available in the Block directly represents the related protobuf.
    // The full data model for an Ethereum Block is available at the following link.
    // https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto
    log::info!("block.ver: {:#?}", block.ver);
    log::info!("block.number: {:#?}", block.number);

    // Copy the data in the Block's version field and return it to caller.
    // Substreams developers will typically pass extracted data through a custom
    // protobuf to a store module.
    Ok(basicexample::BasicExampleProtoData {version: block.ver})
}
```
{% endcode %}

The steps to follow for working with the example include the following:&#x20;

* running the command to generate the required protobufs,
* compiling the example with the Rust compiler,&#x20;
* and sending commands to the Substreams CLI.

At the root of the example project, generate your structs from protobuf specified in the YAML:

{% code overflow="wrap" %}
```bash
substreams protogen substreams-ethereum-tutorial.yaml
```
{% endcode %}

Then compile with:

```shell
cargo build --release --target wasm32-unknown-unknown
```

You're now ready to run:

{% code overflow="wrap" %}
```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams-ethereum-tutorial.yaml map_basic_eth --start-block 1000000 --stop-block +1
```
{% endcode %}

### Solana Example

The procedure for working with the Solana example is very similar to working with the Ethereum example.&#x20;

Clone or download the Solana example codebase. Find the example in the official GitHub repository.

[https://github.com/streamingfast/substreams-solana-tutorial](https://github.com/streamingfast/substreams-solana-tutorial)

After cloning the Solana example, take a moment to review the repository. Key differences from the Ethereum example should begin to stand out immediately.

A very important difference between the two examples is the module handler code. Blockchains each have their own architectures, implementations, and data structures. Blocks for Ethereum or even Bitcoin will be constructed and designed differently. Some of the differences are small and subtle while others are not.

Notice the Solana module handler is accessing the `previous_blockhash` `blockhash` and `slot` fields of the block passed into the module handler by Substreams.&#x20;

The Ethereum example's module handler accessed the `ver` and `number`. These are differences in the block model for each of the blockchains.

****

{% code title="src/lib.rs" overflow="wrap" %}
```rust
#[substreams::handlers::map]
fn map_basic_sol(block: solpb::sol::v1::Block) -> Result<basicexample::BasicExampleProtoData, substreams::errors::Error> {
    // Extract data from the Solana Block and log to the console.
    // The data available in the Block directly represents the related protobuf.
    // The full data model for a Solona Block is available at the following link.
    // https://github.com/streamingfast/firehose-solana/blob/develop/proto/sf/solana/type/v1/type.proto
    log::info!("block.previous_blockhash: {:#?}", block.previous_blockhash);
    log::info!("block.blockhash: {:#?}", block.blockhash);
    log::info!("block.slot: {:#?}", block.slot);

    // Copy the data in the Block's blockhash field and return it to caller.
    // Substreams developers will typically pass extracted data through a custom
    // protobuf to a store module.
    Ok(basicexample::BasicExampleProtoData {blockhash: block.blockhash})
}
```
{% endcode %}

Follow the same steps used for the Ethereum example to work with the Solana example.

When issuing the following command to run the Solana example take note of the different endpoint, map module name, and manifest filename.

{% code overflow="wrap" %}
```basic
substreams run -e mainnet.sol.streamingfast.io:443 substreams-solana-example.yaml sol_basic_mapper --start-block 10000000 --stop-block +1
```
{% endcode %}

## **Next Steps**

The key takeaways at this point are:

1. Substreams is chain-agnostic and can be used with many different blockchains.
2. Block data for each blockchain follows a different structure and model.
3. Each blockchain has a different endpoint associated with it.
4. Each blockchain has a different package associated with it.
5. Custom protobufs are created to pass data from one module to another.

{% hint style="info" %}
**Note**: Gaining a basic understanding of how Substreams works with multiple blockchains will enable developers to graduate to build even more complex solutions.&#x20;
{% endhint %}

Understanding map and store modules is the next step to understanding how to design and craft a fully directed acyclic graph in the Substreams manifest.

Additional information is available for understanding [modules](../concept-and-fundamentals/modules.md), and sample code with explanations can be found in the [Developer Guide](../developer-guide/overview.md).&#x20;

Visit the [Substreams Template](https://github.com/streamingfast/substreams-template) repository and [Substreams Playground](https://github.com/streamingfast/substreams-playground) to get up and running quickly.

## **Troubleshooting & Errors**

### **Requesting Block types from the wrong chain endpoint**

{% code overflow="wrap" %}
```shell
Error: rpc error: code = InvalidArgument desc = validate request: input source "type:\"sf.solana.type.v1.Block\"" not supported, only "sf.ethereum.type.v2.Block" and 'sf.substreams.v1.Clock' are valid
```
{% endcode %}

A common mistake when first getting started is requesting data for one chain, such as Ethereum, and providing an incorrect chain endpoint, for a different blockchain. It's important to note that data from one chain is not compatible with another. The error seen above is informing the developer of this issue.

To resolve the problem, double-check the code and settings within the Substreams codebase against the endpoint that's being sent to the CLI. The error above is from an Ethereum codebase requesting Solana Blocks.&#x20;

The solution is to review the codebase and determine what blockchain the code and configuration require, in contrast to what is being sent to the CLI.
