Tutorial on TRON
===================

In this tutorial, you'll learn how to initialize a TRON-based Substreams project using the Substreams CLI (`substreams init` command).

{% hint style="info" %} 
 The CLI installation is supported only on Linux and macOS. If you're using Windows, consider using the [DevContainer environment](../references/devcontainer-ref.md), which launches a Linux-based virtual environment.
{% endhint %}

## Step 1: Initialize Your TRON Substreams Project

1. [Install the Substreams CLI](../references/cli/installing-the-cli.md)

2. Running `substreams init` will give you the option to choose between two TRON project options. Select the one that best fits your requirements:
    - **tron-hello-world**: Creates a simple Substreams example, which outputs `TransferContract` contracts that have an `amount` above 100M. You can modify this example to extract the specific data that you need.
    - **tron-transactions**: Creates a Substreams that outputs filtered transactions (full transactions). You can filter by `contract_type`, `to`, `from` and `contract_address`.

{% hint style="info" %} 
 The project options above receive transactions as input. Alternatively, Substreams can receive a full TRON Block as input if you need to read data from the Block header.
{% endhint %}
    
## Step 2: Visualize the Data

1. Run `substreams auth` to create your [account](https://thegraph.market/) and generate an authentication token (JWT), then pass this token back as input.

2. Run `substreams build` to compile the project.

3.  Now you can freely use the `substreams gui` to visualize and iterate on your extracted data.

## Step 2.5: (Optionally) Transform the Data 

1. Open the `src/lib.rs` file that has been generated.

2. Modify the transformations made to the data as needed. Every time you modify the code, you will have to compile again the project with `substreams build`.

## Step 3: Load the Data

To make your Substreams queryable (as opposed to [direct streaming](../how-to-guides/sinks/stream/stream.md)), you can automatically send the data to a SQL data by using the [SQL sink](../how-to-guides/sinks/sql/sql.md).

## Additional Resources

You may find these additional resources helpful for developing your first TRON application.

### Dev Container Reference

The [Dev Container Reference](../../references/devcontainer-ref.md), in case you are developing on Windows and need a Linux virtual environment.

### Substreams Components Reference

The [Components Reference](../../references/substreams-components/packages.md) dives deeper into navigating the `substreams.yaml`.
