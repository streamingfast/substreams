The Substreams Development Container (“devcontainer”) is a tool to help you build your first project. You can either run it remotely or clone the [substreams-starter repository](https://github.com/streamingfast/substreams-starter?tab=readme-ov-file) to run it locally. Inside the devcontainer, the `substreams init` command sets up a code-generated Substreams project, allowing you to easily build a subgraph or an SQL-based solution for data handling.

##Prerequisites

- Ensure Docker and VS Code are up-to-date.

## Navigating the Devcontainer

Upon entering the devcontainer, you can either insert your `substreams.yaml` file and run `substreams build` to generate Protobuf files or choose one of the auto-generated paths:

- **Minimal**: Extracts raw data from the block.
- **Non-Minimal**: Extracts filtered data using network-specific cache and Protobufs from the Foundational Modules.

## Building Your Project

You can configure your Substreams project for querying either through a Subgraph or directly from your SQL database:

- **Subgraph**: Run `substreams codegen subgraph`. This generates a project with a basic `schema.graphql` and `mappings.ts` file. You can customize these to define entities based on the data extracted by Substreams. For more information on configuring a Subgraph sink, see the [Subgraph documentation](https://substreams.streamingfast.io/documentation/consume/subgraph).
- **SQL**: Run `substreams codegen sql` for SQL-based queries. For more information on configuring a SQL sink, refer to the [SQL documentation](https://substreams.streamingfast.io/documentation/consume/sql).

## Deployment Options

To deploy a Subgraph, you can either run the `graph-node` locally using the `deploy-local` command or deploy to Subgraph Studio by using the `deploy` command from the `package.json` file.

## Common Errors

- When running locally, make sure to verify that all Docker containers are healthy by running the `dev-status` command. 
