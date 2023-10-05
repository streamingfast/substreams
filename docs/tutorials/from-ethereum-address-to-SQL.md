---
description: Quickly build a Substreams that writes events from a contract to an SQL database. 
---

In this tutorial, you will learn how to bootstrap a Substreams with the Bored Ape Yacht Club contract. You will be going from no code to a fully-fledged environment. You will be able to fill a local SQL database with events emitted on-chain and access from an SQL browser and through GraphQL. 

# Requirements
1. Substreams cli: version v1.1.15 or above required, navigate to [Installing the Cli](https://substreams.streamingfast.io/getting-started/installing-the-cli)
2. Docker: visit the official installation [page](https://docs.docker.com/engine/install/)
3. (Optional) Rust: to better tweak, get your hands dirty and have fun developing substreams, you need to install Rust. Visit the official Rust installation [page](https://www.rust-lang.org/tools/install)

# Generate base Substreams code
{% code overflow="wrap" %}
```bash
$> substreams init
Project name: my-bayc
Protocol: Ethereum
Ethereum chain: Mainnet
Track contract: y # you can track any verified contract
Verified Ethereum contract address to track: 0xbc4ca0eda7647a8ab7c2061c2e118a18a936f13d # Bored Ape Yacht Club
Retrieving Ethereum Mainnet contract information (ABI & creation block)
Writing project files
Generating Protobuf Rust code
Project "my-bayc" initialized at "/absolute/path/to/my-first/substreams/"
```
{% endcode %}

# Fetch your api token
You will first need to get a StreamingFast API **key** from [https://app.streamingfast.io](https://app.streamingfast.io). Using this API key, retrieve an API **token** by using:

{% code overflow="wrap" %}
```bash
export STREAMINGFAST_KEY=server_123123 # Use your own API key
export SUBSTREAMS_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
```
{% endcode %}

# Build your generated Substreams
{% code overflow="wrap" %}
```bash
cd my-bayc/
make build # this will either use your local rust environment if you have "cargo" installed, or a docker container
```
{% endcode %}

# Test your Substreams
{% code overflow="wrap" %}
```bash
substreams run substreams.yaml db_out --substreams-endpoint mainnet.eth.streamingfast.io:443 --stop-block +1
```
{% endcode %}

This is the expected output of your test
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

# Test your Substreams with the GUI
You can also test your Substreams with the GUI. The GUI is a very good debugging tool if you want to have a better view of all your modules and be able to analyze all the outputs.

{% code overflow="wrap" %}
```bash
substreams gui substreams.yaml db_out 
```
{% endcode %}

# Deploy to a local sink dev environment
* Your generated `substreams.yaml` defines your substreams as an SQL deployable unit with the `sink` section.

1. Run the `serve` command in a shell (this is the development server that will create docker containers to run the sink and database)
    {% code overflow="wrap" %}
    ```bash
    $> substreams alpha sink-serve
    2023-10-04T14:31:33.064-0400 INFO (substreams) starting server server
    2023-10-04T14:31:33.064-0400 INFO (substreams) grpc server with plain text server
    2023-10-04T14:31:33.064-0400 INFO (substreams) launching server {"listen_addr": "localhost:8000"}
    2023-10-04T14:31:33.065-0400 INFO (substreams) serving plaintext {"listen_addr": "localhost:8000"}
    ```
    {% endcode %}
2. From another shell, deploy your Substreams and see the output services details:
    {% code overflow="wrap" %}
    ```bash
    $> substreams alpha sink-deploy ./substreams.yaml
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
    {% endcode %} 
3. After a few seconds, the command `substreams alpha sink-info` should give you information about the progress of the sink (ex: `Last processed block: 12722000`)
4. You can check the logs of the sink and see what's happening: `docker logs -f f94fe55c-sink`
5. You can explore the different services directly from your browser:
   * Postgraphile: http://localhost:3000/graphiql
   * PGWeb: http://localhost:8081/

Here is a PBWeb view of events added to **Approval For Alls** Table

<figure>
   <img src="../.gitbook/assets/pgweb.png" />
   <figcaption>
      <p>Approval For Alls PGWeb View</p>
   </figcaption>
</figure>
![img.png](../../.gitbook/assets/pgweb.png/pgweb.png)

Here is the Postgraphile view of the **Approval For Alls**
<figure>
   <img src="../.gitbook/assets/postgraphile.png" />
   <figcaption>
      <p>Approval For Alls Postgraphile View</p>
   </figcaption>
</figure>
