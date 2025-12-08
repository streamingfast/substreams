Tutorial on Monad
==================

In this tutorial, you'll learn how to initialize a Monad-based Substreams project using the Substreams CLI and the available Monad development resources.

{% hint style="warning" %}
Due to Monad full nodes not providing access to arbitrary historic state, eth_calls are unsupported for Monad Substreams packages. See [https://docs.monad.xyz/developer-essentials/historical-data#state](https://docs.monad.xyz/developer-essentials/historical-data#state) for more information.
{% endhint %}

{% hint style="info" %} 
The CLI installation is supported only on Linux and macOS. If you're using Windows, consider using the [DevContainer environment](../references/devcontainer-ref.md), which launches a Linux-based virtual environment.
{% endhint %}

## Step 1: Initialize Your Monad Substreams Project

1. [Install the Substreams CLI](../how-to-guides/cli/installing-the-cli.md).

2. Running `substreams init` will give you the option to choose the EVM protocol, then select one of three project options that best fits your requirements:
   - **evm-hello-world**: Creates a Substreams that extracts a popular ERC20 token's log data from blocks
   - **evm-events-calls-raw**: (without ABI) Get raw Ethereum events/calls and create a Substreams as source. You'll need to specify the contract address(es) that you want to follow.
   - **evm-events-calls**: (with ABI) Decode Ethereum events/calls using an ABI and create a Substreams as source. You'll need to specify the contract address(es) that you want to follow. Contract ABIs are retrieved from [Monadscan](https://monadscan.com/), or you can provide them yourself if not available.

## Step 2: Visualize the Data

1. Run `substreams auth` to create your [account](https://thegraph.market/) and generate an authentication token (JWT), then pass this token back as input.

2. Run `substreams build` to compile the project.

3. Run `substreams gui` to visualize and iterate on your extracted data.

## Step 2.5: (Optionally) Transform the Data 

1. Open the `src/lib.rs` file that has been generated.

2. Modify the transformations made to the data as needed. Every time you modify the code, you will have to recompile the project with `substreams build`.

## Step 3: Load the Data

To make your Substreams queryable (as opposed to [direct streaming](../how-to-guides/sinks/stream/stream.md)), you can automatically send the data to a SQL data by using the [SQL sink](../how-to-guides/sinks/sql/sql.md) or through [PubSub](../how-to-guides/sinks/pubsub.md).

## Additional Resources

You may find these additional resources helpful for developing your first Monad application.

### Monad Development Kit

The [Substreams Ethereum](https://github.com/streamingfast/substreams-ethereum) development kit provides Rust Firehose Block models and helpers specifically for EVM-compatible chains like Monad.

### Monad Endpoints

Monad Substreams are available on the following endpoints:
- **Monad Mainnet**: `mainnet-base.monad.streamingfast.io:443`

### Dev Container Reference

The [Dev Container Reference](../../references/devcontainer-ref.md), in case you are developing on Windows and need a Linux virtual environment.

### Substreams Components Reference

The [Components Reference](../../references/substreams-components/packages.md) dives deeper into navigating the `substreams.yaml`.
