Quickly scaffold your first project inside the [Substreams Development Environment](https://github.com/streamingfast/substreams-starter?tab=readme-ov-file) (”devcontainer”). Check out the Getting Started in the `README`to run remotely or clone the substreams-starter repository to run locally. Inside the devcontainer the `substreams init` command sets up a code-generated Substreams project, from which you can easily build either a subgraph or an SQL-based solution for handling data.

{% hint style="info" %}
**Note:** Validate Docker and VS Code are up-to-date.
{% endhint %}

Consult the relevant ecosystem guide to get started using real-time and historical indexed data:

- Solana
- EVM
- Injective

## Navigatin the Devcontainer

When entering the devcontainer, you can either insert your own `substreams.yaml` file and run `substreams build` to generate the associated Protobuf files, or choose from two auto-generated code-paths:

- **Minimal**: Creates a simple Substreams that extracts raw data from the block.
- **Non-Minimal**: Extracts filtered data specific to the network and relies on the cache and Protobufs provided by the Foundational Modules.

Complete your Substreams project to be fully queryable, either through a Subgraph or directly from your SQL database, ensuring seamless access to the data for analysis and application use:

- `substreams codegen subgraph`: The generated project follows the standard subgraph structure. By default, the `schema.graphql` and `mappings.ts` files respectively include only a required input ID and the basic code to create one. It's up to you to decide what entities to create based on the data extracted by Substreams. For technical details on how to configure a Subgraph sink, [click here](https://substreams.streamingfast.io/documentation/consume/subgraph).
- `substreams codegen sql`: For technical details on how to configure a SQL sink, [click here](https://substreams.streamingfast.io/documentation/consume/sql).

If your plan is to deploy a Subgraph you may choose to either run the `graph-node` locally with the `deploy-local` command or deploy to the Subgraph Studio by checking out the `deploy` command in the `package.json`.

{% hint style="info" %}
**Note:** When running local, make sure to verify that all containers are running properly in the Docker tab and that there’s no errors in the logs.
{% endhint %}