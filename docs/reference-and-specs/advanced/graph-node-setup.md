---
description: StreamingFast graph-node setup
---

# Graph-node setup

## Graph-node setup overview

Substreams and `graph-node` can be used together. Substreams-based subgraphs can be pushed into `graph-node` setups.

#### **StreamingFast graph-node-dev**

Clone the StreamingFast `graph-node-dev` repository on Github:

```bash
git clone git@github.com:streamingfast/graph-node-dev.git
```

<details>

<summary><strong>Docker setup</strong></summary>

Docker is required to use StreamingFast `graph-node-dev`. Make sure your machine has a functional Docker installation in place prior to proceeding.

Additional information for Docker installation can be [found in the official Docker documentation](https://docs.docker.com/engine/install/).

</details>

<details>

<summary><strong>NodeJS, NPM and Yarn setup</strong></summary>

Node Package Manager (NPM) and Yarn are required to use StreamingFast `graph-node-dev`.

Additional information for NodeJS and NPM installation can be [found in the official NPM documentation](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm).

Additional information for Yarn installation can be [found in the official yarn documentation](https://classic.yarnpkg.com/lang/en/docs/install/#mac-stable).

</details>

<details>

<summary>PostgreSQL setup</summary>

PostgreSQL is required to use StreamingFast `graph-node-dev`. Make sure your computer has a fully functional PostgreSQL installation in place prior to proceeding.

Additional information for PostgreSQL installation can be [found in the official PostgreSQL documentation](https://www.postgresql.org/download/).

</details>

#### **NodeJS dependencies**

The use of Substreams and `graph-node` together requires multiple Node.js dependencies. Install the dependencies by using:

```
yarn install
```

#### Database and IPFS node script

To start the Docker containers for PostgreSQL and the IPFS node, run the `up.sh` shell script included in the `graph-node-dev` repository.

{% hint style="info" %}
**Note**: The `-c` flag can be added when running the up.sh shell script to clean any persistent directories for PostgreSQL, IPFS nodes, and other similar services before starting them.
{% endhint %}

```bash
./up.sh
```

#### Optional Firehose services

To test subgraphs pulling data from Firehose a connection must be established based on the network being consumed such as Ethereum or Solana.

Shell scripts are included in the `graph-node-dev` repository to set up the port-forward to the peering services for Ethereum, Binance Smart Chain, and Solana.

The `./pf-eth.sh` shell script starts the services for Ethereum.

```bash
./pf-eth.sh
```

The `./pf-bsc.sh` shell script also starts the services for Binance Smart Chain.

```bash
./pf-bsc.sh
```

The `./pf-sol.sh` shell script starts the services for Solana.

```bash
./pf-sol.sh
```

#### The Graph Protocol `graph-node`

The `graph-node` repository from [The Graph](https://thegraph.com/) is also required to use StreamingFast `graph-node-dev`. Clone the `graph-node` repository on Github:

```bash
git clone https://github.com/graphprotocol/graph-node
```

**Running `graph-node`**

You're now ready to run `graph-node.`

Run `graph-node` from its root directory. The `config/graph-node.eth-ropsten.toml` configuration file references the `graph-node` repository directory. Update the paths accordingly.

{% hint style="info" %}
**Note**: To successfully connect to the Firehose and Substreams services, you might need to set up certain system environment variables, such as `STREAMING_FAST_API_TOKEN`.
{% endhint %}

Start up `graph-node` by using:

{% code overflow="wrap" %}
```bash
GRAPH_LOG=trace cargo run -- --config config/graph-node.eth-ropsten.toml --ipfs "localhost:5001"
```
{% endcode %}

**Subgraph deployment**

The **** subgraph manifest file needs to be pushed to the local IPFS node.&#x20;

Push the subgraph to IPFS by using:

```bash
ipfs add substreams/ethereum/mainnet-network.yaml
```

After the subgraph manifest has been pushed to IPFS the subgraph can be deployed.

{% hint style="info" %}
**Note**: http can be installed by using Homebrew through the httpie command.
{% endhint %}

Deploy the subgraph by using:

```bash
export i=QmUFVjzLeSRAjUNnNcdC4LEM3kncZwand2fj7gbNBjVV4A
http -I post http://localhost:8020/ jsonrpc="2.0" id="1" method="subgraph_create" params:="{\"name\": \""$i"\"}" && http -I post http://localhost:8020/ jsonrpc="2.0" id="1" method="subgraph_deploy" params:="{\"name\": \""$i"\", \"ipfs_hash\": \""$i"\", \"version_label\": \"1\"}"
```

#### `config-firehose.toml`

The `config-firehose.toml` file assumes the dependencies are provided by `docker-compose up`, which is started through `up.sh` invocations.

{% code title="config-firehose.toml" overflow="wrap" lineNumbers="true" %}
```rust
[[general]

[store]
[store.primary]
connection = "postgresql://graph-node:let-me-in@localhost:5432/graph-node"
weight = 1
pool_size = 10

[chains]
ingestor = "block_ingestor_node"
[chains.ropsten]
shard = "primary"
provider = [
  { label = "firehose", details = { type = "firehose", url = "https://ropsten.streamingfast.io", token = "<fill_me>" }},
  { label = "peering", url = "http://localhost:8545", features = [] },
]

[deployment]
[[deployment.rule]]
shard = "primary"
indexers = [ "default" ]
```
{% endcode %}

#### Further information

For more information and setup instructions for The Graph's `graph-node`, [refer to The Graph Academy documentation](https://docs.thegraph.academy/official-docs/indexer/testnet/graph-protocol-testnet-baremetal/3\_deployandconfiguregraphnode).

You can [find more information on subgraphs](https://thegraph.com/docs/en/developing/creating-a-subgraph/) in The Graph's subgraph documentation.

You can [find more information on StreamingFast `graph-node-dev`](https://github.com/streamingfast/graph-node-dev) in the official `graph-node-dev` repository.
