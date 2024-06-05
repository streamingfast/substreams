## Local Development of Subgraphs with Graph Node

The Graph Node is the software that indexers run to index subgraphs. When developing a subgraph (or a Substreams-powered subgrpah), it is very convenient to test the subgraph deployment locally. This can be achieved by running the Graph Node software and all its dependencies in a local Docker environment.

Clone the [Substreams Development Environment GitHub respository](), which contains the necessary shell scripts to run a local Graph Node in your computer.

### Requirements

This tutorial requires you to:
- Have Docker installed.
- Run a Unix-like operating system, so that you can execute bash scripts.

### Set up the Environment

In the [Substreams Development Environment GitHub respository](), move to the `graph-node` folder. The entrypoint to set up the Graph Node local environment is the `start.sh` script, which spins up a Graph Node instance configured for a specific network (e.g. `injective-mainnet`), along with a local IPFS node and a local Postgres database. When using this script, you must pass two parameters: `NETWORK` and `SUBSTREAMS_ENDPOINT`.

```bash
./start.sh <NETWORK> <SUBSTREAMS_ENDPOINT>
```

For example, the following command spins up a Graph Node for the `injective-mainnet` network using the `https://mainnet.injective.streamingfast.io:443` Substreams endpoint.

```bash
./start.sh injective-mainnet https://mainnet.injective.streamingfast.io:443
```

The script also expects the `SUBSTREAMS_API_TOKEN` environment variable to be configured with your Substreams API authentication token.

### Interact With the Graph Node

You can interact with the Graph Node using the Graph CLI (`graph`). Some useful command are:

- `graph build --ipfs=http://localhost:5001`: build the subgraph and store the build files in the local IPFS node.
- `graph create <NAME> --node=http://localhost:8020`: create a new subgraph in the local Graph Node.
- `graph remove <NAME> --node=http://localhost:8020`: remove an existing subgraph in the local Graph Node.
- `graph deploy --node http://localhost:8020/ --ipfs http://localhost:5001 <NAME>`: deploy a subgraph to the local Graph Node.
