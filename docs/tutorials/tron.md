Tutorial on TRON
===================

In this tutorial, you'll learn how to initialize a TRON-based Substreams project using the Substreams CLI (`substreams init` command).

{% hint style="info" %} 
 The CLI installation is supported only on Linux and macOS. If you're using Windows, consider using the [DevContainer environment](../references/devcontainer-ref.md), which launches a Linux-based virtual environment.
{% endhint %}

## Step 1: Initialize Your TRON Substreams Project

1. [Install the Substreams CLI](../how-to-guides/cli/installing-the-cli.md).

2. Running `substreams init` will give you the option to choose between three TRON project options. Select the one that best fits your requirements:
    - **tron-hello-world**: Creates a simple Substreams example, the example outputs results with type `TransferContract` that have an `amount` above 100M. Use this example to learn how to write a custom Substreams starting from the [TRON Full block](https://github.com/streamingfast/firehose-tron/blob/main/proto/sf/tron/type/v1/block.proto) as input.
    - **tron-transactions**: Generates a Substreams that outputs filtered transactions (full transactions). Filtering is supported on `contract_type`, `to`, `from` and `contract_address`.
    - **Tron EVM (mainnet)**: Navigate to the `substreams init` [EVM path](./evm.md) to access TRON-specific EVM data. While the TRON full blocks do contain everything, TRON EVM mainnet contains only the EVM associated transactions, receipts, and logs. 
    
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

You may find these additional resources helpful for developing your first TRON application.

### Dev Container Reference

The [Dev Container Reference](../../references/devcontainer-ref.md), in case you are developing on Windows and need a Linux virtual environment.

### Substreams Components Reference

The [Components Reference](../../references/substreams-components/packages.md) dives deeper into navigating the `substreams.yaml`.
