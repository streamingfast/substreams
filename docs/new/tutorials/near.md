Tutorial on NEAR
===================

In this tutorial, you'll learn how to initialize a NEAR-based Substreams project using the Substreams CLI and the available NEAR development resources.

{% hint style="info" %} 
 The CLI installation is supported only on Linux and macOS. If you're using Windows, consider using the [DevContainer environment](../references/devcontainer-ref.md), which launches a Linux-based virtual environment.
{% endhint %}

## Step 1: Initialize Your NEAR Substreams Project

1. [Install the Substreams CLI](../how-to-guides/cli/installing-the-cli.md).

2. Running `substreams init` will give you the option to choose between NEAR project options. Select the one that best fits your requirements:
   - **near-hello-world**: Creates a simple Substreams example that demonstrates how to extract and process NEAR blockchain data using the [NEAR Full block](https://github.com/streamingfast/firehose-near/blob/develop/proto/sf/near/type/v1/type.proto) as input.

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

You may find these additional resources helpful for developing your first NEAR application.

### NEAR Development Kit

The [Substreams NEAR](https://github.com/streamingfast/substreams-near) development kit provides Rust Firehose Block models and helpers specifically for NEAR chains.

### NEAR Endpoints

NEAR Substreams are available on the following endpoints:
- **NEAR Mainnet**: `mainnet.near.streamingfast.io:443`
- **NEAR Testnet**: `testnet.near.streamingfast.io:443`

### Dev Container Reference

The [Dev Container Reference](../../references/devcontainer-ref.md), in case you are developing on Windows and need a Linux virtual environment.

### Substreams Components Reference

The [Components Reference](../../references/substreams-components/packages.md) dives deeper into navigating the `substreams.yaml`.
