---
title: Getting Started on World Chain
---

In this tutorial, you'll learn how to initialize a World Chain-based Substreams project using the Substreams CLI (`substreams init` command).

{% hint style="info" %}
 The CLI installation is supported only on Linux and macOS. If you're using Windows, consider using the [DevContainer environment](../references/devcontainer-ref.md), which launches a Linux-based virtual environment.
{% endhint %}

{% hint style="info" %}
**Tip**: Using an AI coding assistant? Install the [Substreams agent skills](../how-to-guides/develop-your-own-substreams/generic/agent-skills.md) to get expert Substreams guidance while building.
{% endhint %}

## Step 1: Initialize Your World Chain Substreams Project

1. [Install the Substreams CLI](../how-to-guides/cli/installing-the-cli.md)

2. Running `substreams init` will give you the option to choose between two EVM project options. Select the one that best fits your requirements:
    - **evm-hello-world**: Creates a simple Substreams that outputs the events of a smart contract. For World Chain, this will typically use a common smart contract address available on the network.
    - **evm-events-calls**: Creates a Substreams that extracts and decodes EVM events and calls using the cached [EVM Foundational Module](https://substreams.dev/streamingfast/ethereum-common/v0.3.0), filtered by one or more smart contract addresses. Contract ABIs are retrieved from Etherscan-compatible block explorers. If an ABI isn't available, you'll need to provide it yourself.

## Step 2: Configure Your World Chain Endpoint

When running your Substreams commands, use the World Chain endpoint:

```bash
substreams run -e mainnet.worldchain.streamingfast.io:443 substreams.yaml [module_name] --start-block [block_number]
```

## Step 3: Visualize the Data

1. Run `substreams auth` to create your [account](https://thegraph.market/) and generate an authentication token (JWT), then pass this token back as input.

2. Run `substreams build` to compile the project.

3. Run `substreams gui -e mainnet.worldchain.streamingfast.io:443` to visualize and iterate on your extracted data.

## Step 3.5: (Optionally) Transform the Data

1. Open the `src/lib.rs` file that has been generated.

2. Modify the transformations made to the data as needed. Every time you modify the code, you will have to recompile the project with `substreams build`.

## Step 4: Load the Data

To make your Substreams queryable (as opposed to [direct streaming](../how-to-guides/sinks/stream/stream.md)), you can automatically send the data to a SQL database by using the [SQL sink](../how-to-guides/sinks/sql/sql.md) or through [PubSub](../how-to-guides/sinks/pubsub.md).

## World Chain Specifics

World Chain is an EVM-compatible blockchain, which means:

- It uses the same [`sf.ethereum.type.v2.Block`](https://github.com/streamingfast/firehose-ethereum/blob/develop/proto/sf/ethereum/type/v2/type.proto) protobuf model as other EVM chains
- All EVM-based Substreams modules and patterns work seamlessly
- You can leverage existing EVM foundational modules and libraries
- Smart contract interactions follow standard EVM patterns

## Additional Resources

You may find these additional resources helpful for developing your first World Chain application.

### Dev Container Reference

The [Dev Container Reference](../references/devcontainer-ref.md) helps you navigate the container and its common errors.

### CLI Reference

The [CLI reference](../references/cli/command-line-interface.md) lets you explore all the tools available in the Substreams CLI.

### Substreams Components Reference

The [Components Reference](../references/substreams-components/packages.md) dives deeper into navigating the `substreams.yaml`.

### EVM Development Guide

Since World Chain is EVM-compatible, you can also refer to the general [EVM development guide](../how-to-guides/develop-your-own-substreams/evm/exploring-ethereum/exploring-ethereum.md) for more advanced patterns and techniques.
