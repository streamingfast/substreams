---
description: StreamingFast graph-node-dev setup documentation
---

# Graph-Node setup

## Graph-Node setup

Substreams is capable of working in conjunction with graph-node. In addition, Substreams-based subgraphs can be pushed into graph-node setups.

#### **StreamingFast graph-node dev**

The first step to get up and running with graph-node and Substreams is to clone the StreamingFast graph-node-dev repository on Github.&#x20;

The following command can be used to clone the repo directly.

```
git clone git@github.com:streamingfast/graph-node-dev.git
```

<details>

<summary><strong>Docker setup</strong></summary>

Docker is required to use StreamingFast graph-node-dev. Make sure the target machine has a functional Docker installation in place prior to proceeding.

Additional information for Docker installation can be found in the official Docker documentation.

[https://docs.docker.com/engine/install/](https://docs.docker.com/engine/install/)

</details>

<details>

<summary><strong>NodeJS, NPM and Yarn setup</strong></summary>

Node Package Manager (NPM) and Yarn are required to use StreamingFast graph-node-dev. Links with additional information and setup instructions for both are provided below.

Additional information for NodeJS and NPM installation can be found in the official NPM documentation.

[https://docs.npmjs.com/downloading-and-installing-node-js-and-npm](https://docs.npmjs.com/downloading-and-installing-node-js-and-npm)

Additional information for Yarn installation can be found in the official yarn documentation.

[https://classic.yarnpkg.com/lang/en/docs/install/#mac-stable](https://classic.yarnpkg.com/lang/en/docs/install/#mac-stable)

</details>

<details>

<summary>PostgreSQL Setup</summary>

PostgreSQL is required to use StreamingFast graph-node-dev. Make sure the target machine has a fully functional PostgreSQL installation in place prior to proceeding.

Additional information for PostgreSQL installation can be found in the official PostgreSQL documentation.

[https://www.postgresql.org/download/](https://www.postgresql.org/download/)

</details>

#### **NodeJS dependencies**

Using Substreams with graph-node requires several Node.js dependencies. Start Docker and issue the following command to the terminal window to begin the installation process.

```
yarn install
```

#### Database and IPFS node script

Running the up.sh shell script included in the graph-node-dev repository will start the Docker containers for PostgreSQL and the IPFS node.

{% hint style="info" %}
**Note**: The `-c` flag can be added when running the up.sh shell script to clean any persistent folders associated with PostgreSQL, IPFS nodes, and other similar services before starting them.
{% endhint %}

```
./up.sh
```

#### Optional Firehose services

To test subgraphs pulling data from Firehose a connection must be established based on the network being consumed such as Ethereum or Solana.

Shell scripts are included in the graph-node-dev repository to set up the port-forward to the peering services for Ethereum, Binance Smart Chain, and Solana.

The following shell script starts the services for Ethereum.

```
./pf-eth.sh
```

The following shell script starts the services for Binance Smart Chain.

```
./pf-bsc.sh
```

The following shell script starts the services for Solana.

```
./pf-sol.sh
```

#### The Graph Protocol graph-node

The graph-node repository from The Graph is also required for working with StreamingFast graph-node-dev. The repository can be cloned to the target machine by issuing the following command to the terminal.

```
git clone https://github.com/graphprotocol/graph-node
```

**Running graph-node**

The graph-node is ready to be run at this stage.&#x20;

Run the graph-node from its root directory. The config/graph-node.eth-ropsten.toml configuration file references the graph-node repository directory and the paths need to be updated accordingly.

{% hint style="info" %}
**Note**: Some system environment variables (ENV VARS), such as STREAMING\_FAST\_API\_TOKEN, may need to be setup to successfully connect to the Firehose and Substreams services.&#x20;
{% endhint %}

The following command should be issued using a new terminal window to start up graph-node.

```
GRAPH_LOG=trace cargo run -- --config config/graph-node.eth-ropsten.toml --ipfs "localhost:5001"
```

**Subgraph deployment**

The **** subgraph manifest file needs to be pushed to the local IPFS node.&#x20;

The following command should be issued using a new terminal window to push the subgraph to IPFS.

```
ipfs add substreams/ethereum/mainnet-network.yaml
```

After the subgraph manifest has been pushed to IPFS the subgraph can be deployed.

The following command should be issued using a new terminal window to deploy the subgraph.

{% hint style="info" %}
**Note**: http can be installed using Homebrew with the httpie command.
{% endhint %}

```
export i=QmUFVjzLeSRAjUNnNcdC4LEM3kncZwand2fj7gbNBjVV4A
http -I post http://localhost:8020/ jsonrpc="2.0" id="1" method="subgraph_create" params:="{\"name\": \""$i"\"}" && http -I post http://localhost:8020/ jsonrpc="2.0" id="1" method="subgraph_deploy" params:="{\"name\": \""$i"\", \"ipfs_hash\": \""$i"\", \"version_label\": \"1\"}"
```

#### `config-firehose.toml`

The config-firehose.toml file assumes the dependencies are provided by `docker-compose up` (started through `up.sh` invocations).

```
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

#### Further information

Additional information and setup instructions for The Graph's graph-node can be found in The Graph Academy documentation.

[https://docs.thegraph.academy/official-docs/indexer/testnet/graph-protocol-testnet-baremetal/3\_deployandconfiguregraphnode](https://docs.thegraph.academy/official-docs/indexer/testnet/graph-protocol-testnet-baremetal/3\_deployandconfiguregraphnode)

Additional information on subgraphs can be found in The Graph's subgraph documentation.

[https://thegraph.com/docs/en/developing/creating-a-subgraph/](https://thegraph.com/docs/en/developing/creating-a-subgraph/)

Additional information on StreamingFast graph-node-dev can be found in the official graph-node-dev repository.

[https://github.com/streamingfast/graph-node-dev](https://github.com/streamingfast/graph-node-dev)
