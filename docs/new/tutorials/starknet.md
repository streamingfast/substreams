Tutorial on Starknet
=================

In this tutorial, you'll learn how to initialize a Starknet-based Substreams project using the Substreams CLI (`substreams init` command).

{% hint style="info" %} 
 The CLI installation is supported only on Linux and macOS. If you're using Windows, consider using the [DevContainer environment](../references/devcontainer-ref.md), which launches a Linux-based virtual environment.
{% endhint %}

## Step 1: Initialize Your Starknet Substreams Project

1. [Install the Substreams CLI](../references/cli/installing-the-cli.md).

2. Running `substreams init` will give you the option to choose between two Starknet project options. Select the one that best fits your requirements:
    - **starknet-minimal**: Creates a simple Substreams that extracts raw Starknet block data and generates corresponding Rust code. This path will start you with the full raw block, you can navigate to the `substreams.yaml` (the manifest) to modify the input.
    - **starknet-events**: Creates a Substreams that extracts and decodes Starknet events and calls using the cached [Starknet Foundational Module](https://substreams.dev/packages/starknet-foundational/v0.1.4), filtered by one or more smart contract addresses. Contract ABIs are retrieved from Starkscan. If an ABI isn’t available, you’ll need to provide it yourself.

{% hint style="info" %} 
Note: Starknet ABIs are mutable within blocks, therefore the current ABI of your smart-contract may change in a future block. 
{% endhint %}
    
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

You may find these additional resources helpful for developing your first Starknet application.

### Dev Container Reference

The [Dev Container Reference](../references/devcontainer-ref.md) helps you navigate the container and its common errors. 

### CLI Reference

The [CLI reference](../references/cli/command-line-interface.md) lets you explore all the tools available in the Substreams CLI.

### Substreams Components Reference

The [Components Reference](../references/substreams-components/) dives deeper into navigating the `substreams.yaml`.
