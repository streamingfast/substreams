---
description: >-
  Quickly build a Substreams that writes events from a contract to an SQL
  database.
---

# From Ethereum address to SQL

In this tutorial, you will learn how to bootstrap a Substreams with the Bored Ape Yacht Club contract. You will be going from no code to a fully-fledged environment. You will be able to fill a local SQL database with events emitted on-chain and access from an SQL browser and through GraphQL.

## Requirements

1. Substreams CLI: version v1.1.15 or above required, navigate to [Installing the Cli](https://substreams.streamingfast.io/getting-started/installing-the-cli)
2. Docker: visit the official installation [page](https://docs.docker.com/engine/install/)
3. buf.build CLI: visit the official installation [page](https://buf.build/product/cli)
4. (Optional) Rust: to better tweak, get your hands dirty and have fun developing Substreams, you need to install Rust. Visit the official Rust installation [page](https://www.rust-lang.org/tools/install)

## Generate base Substreams code

Use the `substreams init` command you scaffold a basic Substreams project.

{% code overflow="wrap" %}
```bash
substreams init
```

This commnad will ask you to provide some inputs, such as the name of the project, or the smart contract address that you want to index. If you do not provide a specific smart contract address, the command will create a Substreams template for the "Bored Ape Yacht Club" smart contract. In this example, let's leave that field blank and use the default smart contract.

```bash
Project name (lowercase, numbers, undescores): mybayc
Protocol: Ethereum
Ethereum chain: Mainnet
Contract address to track (leave empty to use "Bored Ape Yacht Club"):
Retrieving Ethereum Mainnet contract information (ABI & creation block)
Fetched contract ABI for bc4ca0eda7647a8ab7c2061c2e118a18a936f13d
Fetched initial block 12287507 for bc4ca0eda7647a8ab7c2061c2e118a18a936f13d (lowest 12287507)
Generating ABI Event models for
  Generating ABI Events for Approval (owner,approved,tokenId)
  Generating ABI Events for ApprovalForAll (owner,operator,approved)
  Generating ABI Events for OwnershipTransferred (previousOwner,newOwner)
  Generating ABI Events for Transfer (from,to,tokenId)
Writing project files
Generating Protobuf Rust code
Project "mybayc" initialized at "/Users/stepd"

Run 'make build' to build the wasm code.

The following substreams.yaml files have been created with different sink targets:
 * substreams.yaml: no sink target
 * substreams.sql.yaml: PostgreSQL sink
 * substreams.clickhouse.yaml: Clickhouse sink
 * substreams.subgraph.yaml: Sink into Substreams-based subgraph
```


{% endcode %}

## Get an API key

You will first need to get a StreamingFast API **key** from [https://app.streamingfast.io](https://app.streamingfast.io). Set it in your SUBSTREAMS_API_KEY environment variable.

{% code overflow="wrap" %}
```bash
export SUBSTREAMS_API_KEY=server_123123 # Use your own API key
```
{% endcode %}

## Build your generated Substreams

{% code overflow="wrap" %}
```bash
cd my-bayc/
make build # uses local "Cargo" command or falls back to using docker
# ... Finished release [optimized] target(s) in 34.13s
```
{% endcode %}

## Test your Substreams

{% code overflow="wrap" %}
```bash
substreams run substreams.yaml db_out --substreams-endpoint mainnet.eth.streamingfast.io:443 --stop-block +1
```
{% endcode %}

This is the expected output of your test:

{% code overflow="wrap" %}
```json
{
  "@module": "db_out",
  "@block": 12287507,
  "@type": "sf.substreams.sink.database.v1.DatabaseChanges",
  "@data": {
    "tableChanges": [
      {
        "table": "ownership_transferreds",
        "pk": "22199329b0aa1aa68902a78e3b32ca327c872fab166c7a2838273de6ad383eba-249",
          ...
          {
            "name": "new_owner",
            "newValue": "aba7161a7fb69c88e16ed9f455ce62b791ee4d03"
          },
          ...
        ]
      }
    ]
  }
}
```
{% endcode %}

{% hint style="info" %}
**Note**: You can also test your Substreams with the GUI, which is a better debugging tool than the run command. Try:

`substreams gui substreams.yaml db_out --substreams-endpoint mainnet.eth.streamingfast.io:443 --stop-block +1`
{% endhint %}

## Deploy to a local sink dev environment

* Your generated `substreams.yaml` defines your substreams as an SQL deployable unit with the `sink` section.

1. Run the `serve` command in a shell (this is the development server that will create docker containers to run the sink and database)

{% code overflow="wrap" %}
```bash
substreams alpha sink-serve
```
{% endcode %}

The previous command will start a daemon that will run in your computer:

```
2023-10-04T14:31:33.064-0400 INFO (substreams) starting server server
2023-10-04T14:31:33.064-0400 INFO (substreams) grpc server with plain text server
2023-10-04T14:31:33.064-0400 INFO (substreams) launching server {"listen_addr": "localhost:8000"}
2023-10-04T14:31:33.065-0400 INFO (substreams) serving plaintext {"listen_addr": "localhost:8000"}
```

2. From another shell, deploy your Substreams:

{% code overflow="wrap" %}
```bash
substreams alpha sink-deploy ./substreams.sql.yaml
```
{% endcode %}

{% hint style="info" %}
**Note**: You can also deploy clickhouse-based substreams by using `substreams.clickhouse.yaml`, or a substreams-based-subgraph by using `substreams.subgraph.yaml`
{% endhint %}

The previous command will deploy your Substreams and send the data to the local PostgreSQL database, providing some information:

```bash
Response for deployment "f94fe55c":
Name: my_bayc (v0.1.0)
Output module: db_out (b9e2beff5403fcc89b14622f8224a72a9de27921)
Status: RUNNING ()
Last processed block: 12942000
Services:
  - f94fe55c-pgweb: PGWeb service "f94fe55c-pgweb" available at URL: 'http://localhost:8081'
  - f94fe55c-postgraphile: Postgraphile service "f94fe55c-postgraphile" available at URL: 'http://localhost:3000/graphiql' (API at 'http://localhost:3000/graphql')
  - f94fe55c-postgres: PostgreSQL service "f94fe55c-postgres" available at DSN: 'postgres://dev-node:insecure-change-me-in-prod@localhost:5432/dev-node?sslmode=disable'
  - f94fe55c-sink: Sink service (no exposed port). Use 'substreams alpha sink-info f94fe55c-sink' to see last processed block or 'docker logs f94fe55c-sink' to see the logs.
```

3. After a few seconds, the command `substreams alpha sink-info` should give you information about the progress of the sink (ex: `Last processed block: 12722000`)

4. You can check the logs of the sink and see what's happening: `docker logs -f f94fe55c-sink`

5. You can explore the different services directly from your browser:

* Postgraphile: http://localhost:3000/graphiql
* PGWeb: http://localhost:8081/

Here is a PGWeb view of events added to **Approval For Alls** Table

<figure><img src="../.gitbook/assets/pgweb.png" alt=""><figcaption><p>Approval For Alls PGWeb View</p></figcaption></figure>

Here is the Postgraphile view of the **Approval For Alls**

<figure><img src="../.gitbook/assets/postgraphile.png" alt=""><figcaption><p>Approval For Alls Postgraphile View</p></figcaption></figure>
