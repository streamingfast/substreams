Tutorial on Solana
===================

In this tutorial, you'll learn how to initialize a Solana-based Substreams project within the Dev Container.

{% hint style="info" %} 
 If you prefer to begin locally within your terminal rather than through the Dev Container (VS Code required), refer to the [Substreams CLI installation guide](../../references/cli/installing-the-cli.md).
{% endhint %}

## Step 1: Initialize Your Solana Substreams Project

1. Open the [Dev Container](https://github.com/streamingfast/substreams-starter) and follow the on-screen steps to initialize your project.

2. Running `substreams init` will give you the option to choose between two Solana project options. Select the one that best fits your requirements:
    - **sol-minimal**: Creates a simple Substreams that extracts raw Solana block data and generates corresponding Rust code. This path will start you with the full raw block, you can navigate to the `substreams.yaml` (the manifest) to modify the input.
    - **sol-transactions**: Creates a Substreams that filters Solana transactions based on one or more Program IDs and/or Account IDs, using the cached [Solana Foundational Module](https://substreams.dev/streamingfast/solana-common/v0.3.0).
    - **sol-anchor-beta**: Given an Anchor IDL, create a Substreams that decodes instructions and events. If an IDL isn’t available using the `idl` subcommand within the [Anchor CLI](https://www.anchor-lang.com/docs/cli), you’ll need to provide it yourself.

The modules within Solana Common exclude voting transactions, to benefit from a 75% reduction in data processing size and costs, delay your stream by over 1000 blocks from head. This can be done using the [`sleep`](https://doc.rust-lang.org/std/thread/fn.sleep.html) function in Rust.

{% hint style="info" %} 
 To access voting transactions, use the full Solana block, `sf.solana.type.v1.Block`, as input.
{% endhint %}
    
## Step 2: Visualize the Data

1. Run `substreams auth` to create your [account](https://thegraph.market/) and generate an authentication token (JWT), then pass this token back as input.

2. Now you can freely use the `substreams gui` to visualize and iterate on your extracted data.

## Step 2.5: (Optionally) Transform the Data 

Within the generated directories, modify your Substreams modules to include additional filters, aggregations, and transformations, then update the manifest accordingly. To learn more about this, visit the [How-to-Guides](../../how-to-guides/develop-your-own-substreams/solana/solana.md)
Within the generated directories, modify your Substreams modules to include additional filters, aggregations, and transformations, then update the manifest accordingly. To learn more about this, visit the [How-to-Guides](../../how-to-guides/develop-your-own-substreams/solana/solana.md)

## Step 3: Load the Data

To make your Substreams queryable (as opposed to [direct streaming](../how-to-guides/sinks/stream/stream.md)), you can automatically generate a SQL sink.

### SQL

1. Run `substreams codegen sql` and choose from either ClickHouse or Postgres to initialize the sink, producing the necessary files. 
2. Run `substreams build` build the [Substreams:SQL](../../how-to-guides/sinks/sql/sql-sink.md) sink. 
3. Run `substreams-sink-sql` to sink the data into your selected SQL DB.

{% hint style="info" %}
**Note**: Run `help` to better navigate the development environment and check the health of containers. 
{% endhint %}

## Additional Resources

You may find these additional resources helpful for developing your first Solana application.

### Dev Container Reference

The [Dev Container Reference](../../references/devcontainer-ref.md) helps you navigate the container and its common errors. 

### CLI Reference

The [CLI reference](../../references/cli/command-line-interface.md) lets you explore all the tools available in the Substreams CLI.

### Substreams Components Reference

The [Components Reference](../../references/substreams-components/packages.md) dives deeper into navigating the `substreams.yaml`.
