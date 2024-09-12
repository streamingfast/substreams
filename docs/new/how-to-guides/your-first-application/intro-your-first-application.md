Getting started with Substreams is simple using the code-generation tools provided by the substreams init command. This command sets up an code-generated Substreams project, from which you can easily build either a subgraph or an SQL-based solution for handling data. The generated framework streamlines the process, allowing you to quickly get up and running with Substreams.

Substreams supports several blockchains, but the process should be similar for all them.

<figure><img src="../../.gitbook/assets/chains-endpoints.png" alt="" width="100%"><figcaption><p>Protobuf for the different supported chains</p></figcaption></figure>

## Run the Development Environment (Devcontainer)

To facilitate the development of Substreams, a local or remote development environment for VSCode is available.

{% hint style="info" %}
**Note:** If you run the Development Environment locally, you must have Docker installed and running.
{% endhint %}

- To run it **locally**, clone and run the [substreams-starter](https://github.com/streamingfast/substreams-starter) project.
- To run it **remotely** (a VSCode code instance in your browser), open [this link](https://github.com/codespaces/new/streamingfast/substreams-starter?machine=standardLinux32gb) (`https://github.com/codespaces/new/streamingfast/substreams-starter?machine=standardLinux32gb`) in your browser. Log in with GitHub and you will get access to a VSCode instance in your browser.

## Generate the Substreams Project

The first step in creating a Substreams project is to scaffold it and generate a Substreams package (`.spkg` file), a binary file that contains the definitions of which data you want to extract from the blockchain:

1. Run `substreams init` to view project initialization options.
2. Choose your preferred Substreams project code path:
    - **Minimal**: Creates a simple Substreams that extracts raw data from the block (generates Rust code).
    - **Non-Minimal**: Extracts filtered data specific to the network (does **not** generate Rust code; relies on Foundational Modules).
3. Provide additional details, such as project name and input to filter the data (for example, in EVM chains, you will provide a smart contract address, while in Solana you will input a Program ID to do the filtering).
4. Once all questions are answered, the Substreams project will be generated in the specified folder.
5. Follow the instructions to build, authenticate, and test in the GUI your Substreams project.
6. Complete your Substreams project to be fully queryable, either through a Subgraph or directly from your SQL database, ensuring seamless access to the data for analysis and application use.
    - `substreams codegen subgraph`: The generated project follows the standard subgraph structure, where the `subgraph.yaml` file uses a Substreams package as its data source. By default, the `schema.graphql` and `mappings.ts` files respectively include only a required input ID and the basic code to create one. It's up to you to decide what entities to create based on the data extracted by Substreams. For technical details on how to configure a Subgraph sink, [click here](https://substreams.streamingfast.io/documentation/consume/subgraph).
    - `substreams codegen sql`: For technical details on how to configure a SQL sink, [click here](https://substreams.streamingfast.io/documentation/consume/sql).


Please, refer to the corresponding _getting started_ tutorial depending on your needs:

- [EVM-based blockchains](evm.md)
- [Solana](solana.md)
- [Injective](injective.md)

**Tips:** 

- To run the Devcontainer locally, have the most up to date version of VS Code and make sure that Docker is running.
- If you’re running the Devcontainer locally and want to create a new project on a different ecosystem we recommend clearing the containers in Docker and running `git clean -xfd` on the cloned `substreams-starter` repo.
- If you decide to change `substreams.yaml` to include some additional logic, re-run `substreams build` to generate the associated Protobuf files.
- When developing Substreams-powered Subgraphs, if you decide to edit the `mappings.ts` and `schema.graphql` re-build your project with `npm run build`.
- You can deploy the subgraph to the Subgraph Studio by checking out the `deploy` command in the `package.json`.
- If you follow the subgraph codegen flow and choose to run the `graph-node` locally with the `deploy-local` command, make sure to verify that all containers are running properly in the Docker tab in VS Code, and that there are no errors in the logs (right click on the `graph-node` container).