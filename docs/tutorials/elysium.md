---
title: Getting Started on Elysium
---

In this tutorial, you'll learn how to initialize an Elysium-based Substreams project using the Substreams CLI (`substreams init` command).

{% hint style="info" %}
The CLI installation is supported only on Linux and macOS. If you're using Windows, consider using the [DevContainer environment](../references/devcontainer-ref.md), which launches a Linux-based virtual environment.
{% endhint %}

{% hint style="info" %}
**Tip**: Using an AI coding assistant? Install the [Substreams agent skills](../how-to-guides/develop-your-own-substreams/generic/agent-skills.md) to get expert Substreams guidance while building.
{% endhint %}

## Step 1: Initialize Your Elysium Substreams Project

1. [Install the Substreams CLI](../how-to-guides/cli/installing-the-cli.md).

2. Running `substreams init` will give you the option to choose the EVM protocol, then select one of the project options that best fits your requirements:
    - **evm-hello-world**: Creates a simple Substreams that outputs the events of a smart contract.
    - **evm-events-calls-raw**: (without ABI) Get raw EVM events/calls and create a Substreams as source. You'll need to specify the contract address(es) that you want to follow.
    - **evm-events-calls**: (with ABI) Decode EVM events/calls using an ABI and create a Substreams as source. You'll need to specify the contract address(es) that you want to follow. Elysium doesn't have a block explorer integration yet, so you'll need to provide the contract ABI yourself.

## Step 2: Configure Your Elysium Endpoint

When running your Substreams commands, use the Elysium endpoint:

```bash
substreams run -e mainnet.elysium.streamingfast.io:443 substreams.yaml [module_name] --start-block [block_number]
```

## Step 3: Visualize the Data

1. Run `substreams auth` to create your [account](https://thegraph.market/) and generate an authentication token (JWT), then pass this token back as input.

2. Run `substreams build` to compile the project.

3. Run `substreams gui -e mainnet.elysium.streamingfast.io:443` to visualize and iterate on your extracted data.

## Step 3.5: (Optionally) Transform the Data

1. Open the `src/lib.rs` file that has been generated.

2. Modify the transformations made to the data as needed. Every time you modify the code, you will have to recompile the project with `substreams build`.

## Step 4: Load the Data

To make your Substreams queryable (as opposed to [direct streaming](../how-to-guides/sinks/stream/stream.md)), you can automatically send the data to a SQL database by using the [SQL sink](../how-to-guides/sinks/sql/sql.md) or through [PubSub](../how-to-guides/sinks/pubsub.md).

## Elysium Specifics

Elysium is an EVM-compatible blockchain built on Arbitrum Nitro, which means:

- It uses the same [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto) protobuf model as other EVM chains
- All EVM-based Substreams modules and patterns work seamlessly
- You can leverage existing EVM foundational modules and libraries
- Blocks are [Extended Blocks](../references/chains-and-endpoints.md#evm-extended-vs-base-block-model), so rich data such as internal calls, balance changes, and storage changes is available
- `eth_call` is supported in Substreams modules

## Additional Resources

### Elysium Endpoints

Elysium Substreams are available on the following endpoint:
- **Elysium Mainnet**: `mainnet.elysium.streamingfast.io:443`

### Dev Container Reference

The [Dev Container Reference](../references/devcontainer-ref.md) helps you navigate the container and its common errors.

### CLI Reference

The [CLI reference](../references/cli/command-line-interface.md) lets you explore all the tools available in the Substreams CLI.

### Substreams Components Reference

The [Components Reference](../references/substreams-components/packages.md) dives deeper into navigating the `substreams.yaml`.

### EVM Development Guide

Since Elysium is EVM-compatible, you can also refer to the general [EVM development guide](../how-to-guides/develop-your-own-substreams/evm/exploring-ethereum/exploring-ethereum.md) for more advanced patterns and techniques.
