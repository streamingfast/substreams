Check out the [Getting Started Guide](./intro-your-first-application.md) for more information on how to initialize your project. There are two options within `substreams init` to initialize your EVM Substreams:

- `evm-minimal`: creates a simple Substreams that extracts raw data from the block (generates Rust code).
- `evm-events-calls`: creates a Substreams that extracts EVM events and calls filtered by one or several smart contract addresses.

## EVM Foundational Modules

The `evm-events-calls` codegen path relies on one of the [EVM Foundational Modules](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/ethereum-common). A Foundational Module extracts the most relevant data from blockchain, so that you don't have to code it yourself.

Specifically, the `evm-events-calls` path uses the [filtered_events_and_calls](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/ethereum-common/substreams.yaml#L142) module from the EVM Foundational Modules to retrieve all the events filtered by specific smart contract addresses.

<figure><img src="../../../.gitbook/assets/intro/ethereum-logo.png" width="100%" /></figure>



