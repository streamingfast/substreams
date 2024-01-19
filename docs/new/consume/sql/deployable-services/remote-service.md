Using the StreamingFast SQL remote (hosted) service is the easiest way to get started with Substreams:SQL. The following tutorial teaches you how to deploy a Substreams package from the [Substreams Registry](https://substreams.dev) to StreamingFast remote service.

{% hint style="success" %}
This tutorial shows you how to deploy Substreams package to the **StreamingFast remote service**.
You can also set up a Substreams deployable service environment in your computer, which will allow you to test your deployable services easily.

The Substreams:SQL Tutorial
{% endhint %}

## Tutorial

In this short tutorial, you will:

- Deploy the Substreams package to the StreamingFast SQL remote service.
- Explore the SQL database in your browser.

Before you get started, make sure you have:
- The Substreams CLI installed.

The package used in this tutorial is the USDT Ethereum package, which retrieves events from the USDT smart contract on the Ethereum blockchain.

### Deploying a Substreams Package to the Remote Service

1. You can find the USDT Ethereum package in the Substreams Registry.

3. Deploy the package using the `substreams alpha service deploy` command.

```bash
substreams alpha service deploy https://spkg.io/enoldev/substreams-ethereum-usdt-v0.1.0.spkg -e https://deploy.streamingfast.io
```

* The `substreams alpha service deploy` command is used to deploy a package to the remote service.
* In this example, you deploy the USDT Ethereum package (`https://spkg.io/enoldev/substreams-ethereum-usdt-v0.1.0.spkg`).
* The `-e` flag specifies the location of the remote service. In this example, you are using the StreamingFast Remote Service (`https://deploy.streamingfast.io`).

The deployment of the package might take 1-2 minutes. 

4. After the deployment is completed, some useful data will be displayed:

```bash
Deploying... (creating services, please wait)
Deployed substreams sink "60589e45":
  Status: RUNNING
Deployment *60589e45* is *RUNNING*
  - Database type is *postgres*
  - Owner is *0qeru2bd28b954a35c12e*

Indexed blocks: [4634748 - 4785000]
  - Sink status is available at 'https://srv.streamingfast.io/60589e45/sinkinfo'

Environment is *Development*, which means:
  - Read-only direct access to the database is available at 'postgresql://db.srv.streamingfast.io:17441/substreams?sslmode=disable&user=dev-node-ro&password=JWgg68gP33lZ'
  - Read/write direct access to the database is available at 'postgresql://db.srv.streamingfast.io:17441/substreams?sslmode=disable&user=dev-node&password=iESYNNa5EihR'
  - Read/write access to the database via PGWeb at 'https://srv.streamingfast.io/60589e45/pgweb'

Postgraphile is available at these URL:
  - GraphiQL (browser): 'https://srv.streamingfast.io/60589e45/graphiql'
  - GraphQL (apps): 'https://srv.streamingfast.io/60589e45/graphql'

Documentation: https://substreams.streamingfast.io
Services:
  - pgweb: pod running
  - postgraphile: pod running
  - postgres: pod running
  - sink: pod running
  - sinkinfo: pod running
```

1. **Service ID:** the identifier of the deployed service, which you can use to manage the service.
2. **URL of the service status:** use this URL to verify the status of the service.
3. **URL of the PostgreSQL client:** use this client to run SQL queries, update the SQL schema, and manage the SQL database in general.
4. **URL of the GraphQL clent:** use this client to run GraphQL queries.

### Inspecting the PostgreSQL client

Once the package is deployed, your Substreams starts indexing data. You can access this data through in the form of a PostgreSQL database or a GraphQL API. To access the PostgreSQL client (PGWeb), copy the URL provided.

<figure><img src="../../../.gitbook/assets/sql/explore-pgweb.png" width="100%" /></figure>

### Stopping a Service

If you have not included a stop block in the package manifest, your Substreams will keep indexing the head of the chain, thus consuming bytes from your StreamingFast plan. To stop a service, get the service ID and run the following command:

```bash
substreams alpha service stop <SERVICE-ID> -e https://deploy.streamingfast.io
```