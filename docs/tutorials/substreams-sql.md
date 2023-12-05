# Substreams:SQL Deployable Service Tutorial

In this tutorial, you will:

1. Generate a simple Substreams that extract all events from the Cryptopunks smart contract on Ethereum.
2. Feed your data into a local PostgreSQL database in a Docker-based development environment.
3. Develop and apply SQL transformations with [dbt models](https://docs.getdbt.com/docs/build/models).
4. Go from a local Substreams:SQL environment to a shared hosted development environment.
5. Create a production `.spkg` package and test it locally
6. Deploy your `.spkg` to a production environment that fills a PostgreSQL database from your Substreams and applies dbt transformations automatically.

## Generate a Substreams project from the Cryptopunks ABI

* The cryptopunks address on mainnet is `0xb47e3cd837ddf8e4c57f05d70ab865de6e193bbb`

* Use the `substreams init` command to fetch the ABI from etherscan and scaffold a Substreams project:

```bash
substreams init
```

Fill the requested information (name: `cryptopunks`, protocol: `ethereum`, chain: `mainnet`, contract: `b47e3cd837ddf8e4c57f05d70ab865de6e193bbb`)
```
Project name (lowercase, numbers, undescores): cryptopunks
Protocol: Ethereum
Ethereum chain: Mainnet
✔ Contract address to track: b47e3cd837ddf8e4c57f05d70ab865de6e193bbb
✔ Would you like to track another contract? (Leave empty if not): █
Would you like to track another contract? (Leave empty if not):
Retrieving Ethereum Mainnet contract information (ABI & creation block)
Fetched contract ABI for b47e3cd837ddf8e4c57f05d70ab865de6e193bbb
Fetched initial block 3914495 for b47e3cd837ddf8e4c57f05d70ab865de6e193bbb (lowest 3914495)
Generating ABI Event models for
  Generating ABI Events for Assign (to,punkIndex)
  Generating ABI Events for PunkBidEntered (punkIndex,value,fromAddress)
  Generating ABI Events for PunkBidWithdrawn (punkIndex,value,fromAddress)
  Generating ABI Events for PunkBought (punkIndex,value,fromAddress,toAddress)
  Generating ABI Events for PunkNoLongerForSale (punkIndex)
  Generating ABI Events for PunkOffered (punkIndex,minValue,toAddress)
  Generating ABI Events for PunkTransfer (from,to,punkIndex)
  Generating ABI Events for Transfer (from,to,value)
Writing project files
Generating Protobuf Rust code
Project "cryptopunks" initialized at "/Users/stepd/repos"

Run 'make build' to build the wasm code.

The following substreams.yaml files have been created with different sink targets:
 * substreams.yaml: no sink target
 * substreams.sql.yaml: PostgreSQL sink
 * substreams.clickhouse.yaml: Clickhouse sink
 * substreams.subgraph.yaml: Sink into Substreams-based subgraph
```

* See the list of events that have been identified above. By default, each event type is mapped to its own table. This happens in the `map_events()` function inside `src/lib.rs` and the SQL schema is available in `schema.sql`.

* Build the rust code:

```bash
make build
```

You should see:

```bash
cargo build --target wasm32-unknown-unknown --release
   Compiling proc-macro2 v1.0.69
   Compiling unicode-ident v1.0.12
   Compiling syn v1.0.109
   ...
   Compiling cryptopunks v0.0.1 (/Users/stepd/repos/cryptopunks)
    Finished release [optimized] target(s) in 30.71s
```

## Start feeding a local database using 'substreams alpha service' and Docker

From another window:

```
SUBSTREAMS_API_TOKEN=(...) substreams alpha service serve
```

Back to your substreams project:

* Deploy your substreams locally to start putting data in your database:

```bash
substreams alpha service deploy substreams.sql.yaml
```

{% hint style="success" %}
**Tip:** A new PostgreSQL container will be created at port `5432`. The deployment will fail if there is another Docker container using that port.
{% endhint %}

{% hint style="info" %}
**Tip:** You can also use `substreams service deploy substreams.clickhouse.yaml` to use the Clickhouse engine instead of PostgreSQL. There is no postgraphile or pgweb in that case, you will need a tool like DataGrip to see the data.
{% endhint %}

You should see:

```bash
Deploying... (creating services, please wait)
Deployed substreams sink "7590fdbf":
  Status: STOPPED
Running your deployment inside local docker containersServices:
  - 7590fdbf-pgweb: PGWeb service "7590fdbf-pgweb" available at URL: 'http://localhost:8081'
  - 7590fdbf-postgraphile: Postgraphile service "7590fdbf-postgraphile" available at URL: 'http://localhost:3000/graphiql' (API at 'http://localhost:3000/graphql')
  - 7590fdbf-postgres: PostgreSQL service "7590fdbf-postgres" available at DSN: 'postgres://dev-node:insecure-change-me-in-prod@localhost:5432/substreams?sslmode=disable'
  - 7590fdbf-sink: Sink service (no exposed port). Use 'substreams alpha sink-info 7590fdbf' to see last processed block or 'docker logs 7590fdbf-sink' to see the logs.
  - 7590fdbf-sinkinfo: Sink info service "7590fdbf-sinkinfo" available at URL: 'http://localhost:8282/sinkinfo'
```

* Look at some SQL data via pgweb at [http://localhost:8081](http://localhost:8081)

{% hint style="success" %}
**Tip:** You can run `substreams alpha service pause` if you want to pause the sink from consuming Substreams data while you continue your development. `substreams alpha service resume` will continue the progress.
{% endhint %}

## Create a dbt project for transformations

* Initialize the dbt project:

```bash
dbt init cryptopunks
```

{% hint style="success" %}
**Tip:** Choose 'postgres' as the database.
{% endhint %}

* Move that project under 'dbt' folder:

```bash
mv cryptopunks dbt
```
* Edit the dev credentials under $HOME/.dbt/profiles.yml file:

```bash
...
cryptopunks:
  outputs:
    dev:
      type: postgres
      host: localhost
      user: dev-node
      password: insecure-change-me-in-prod
      port: 5432
      dbname: substreams
      schema: public
...
  target: dev

```

{% hint style="success" %}
**Tip:** You can see the database credentials again by running `substreams alpha service info`
{% endhint %}

* Test the dbt connection

```bash
dbt debug
```

You should see:

```bash
16:04:10  All checks passed!
```

* Remove the example models:

```bash
rm -rf models/example
```

* Create a datasource and a basic materialized view:

```bash
mkdir models/default

cat <<EOF > models/default/source.yml
version: 2

sources:
  - name: cryptopunks
    loaded_at_field: evt_block_time
    tables:
      - name: punk_bought
      - name: punk_bid_entered
      - name: punk_offered
      - name: punk_no_longer_for_sale
      - name: punk_transfer
      - name: punk_bid_withdrawn
      - name: transfer
      - name: assign
EOF

cat <<EOF > models/default/punks_bought_per_hour.sql
{{ config(materialized='table') }}

select date_trunc('hour', evt_block_time) AS "hour", count(*) from punk_bought  GROUP BY hour order by hour
EOF
```

* Create your views:

```bash
dbt run
```

* Check that the `punks_bought_per_hour` has been created.

## Deploy your Substreams to the "hosted dev" environment

Deployments of type "dev" gives you full read/write access to the database and are a good way of working together on a dataset while developing your dbt models.

```bash
substreams alpha service deploy substreams.sql.yaml -e https://deploy.streamingfast.io
```

{% hint style="info" %}
**Tip:** Here again, you can use the `substreams.clickhouse.yaml` manifest to use a Clickhouse engine.
{% endhint %}

You should see this output:

```bash
Deployed substreams sink "54546874":
  Status: RUNNING
Deployment *54546874* is *RUNNING*
  - Database type is *postgres*
  - Owner is *0doqed628575ba7d2bd03*

Indexed blocks: [12287507 - 12326000]
  - Sink status is available at 'https://srv.streamingfast.io/54546874/sinkinfo'

Environment is *Development*, which means:
  - Read-only direct access to the database is available at 'postgresql://db.srv.streamingfast.io:17622/substreams?sslmode=disable&user=dev-node-ro&password=iv2rqsKsUVH5'
  - Read/write direct access to the database is available at 'postgresql://db.srv.streamingfast.io:17622/substreams?sslmode=disable&user=dev-node&password=oTWPmz2Sqssb'
  - Read/write access to the database via PGWeb at 'https://srv.streamingfast.io/54546874/pgweb'

Postgraphile is available at these URL:
  - GraphiQL (browser): 'https://srv.streamingfast.io/54546874/graphiql'
  - GraphQL (apps): 'https://srv.streamingfast.io/54546874/graphql'

See some tutorials at https://substreams.streamingfast.io
```

{% hint style="success" %}
**Tip:** You are getting SQL credentials in this output. They can be fetched again by running `substreams alpha service info 46f5e9f6 -e https://deploy.streamingfast.io`.
{% endhint %}

{% hint style="success" %}
**Tip:** The SQL endpoint will be assigned a specific port mapped to your deployment, in this example, port 17622.
{% endhint %}

* Create another target in your `$HOME/.dbt/profiles.yml` replacing `PORT_NUMBER` and `RW_PASSWORD` with those assigned to you on the previous step:

```bash
cryptopunks:
  outputs:
...
    remote:
      type: postgres
      host: db.srv.streamingfast.io
      user: dev-node
      password: {RW_PASSWORD}
      port: {PORT_NUMBER}
      dbname: substreams
      schema: public
```

* You can iterate using `dbt run --target=remote`
* Other devs can work on that data and create dbt models.

## Pack your production package and test it locally

When your dbt models are ready, you can pack everything (Substreams, [dbt project](https://docs.getdbt.com/docs/build/projects), etc.) inside an `.spkg` file and deploy it as production:

* Add a `dbt_config` section to the `substreams.sql.yaml` (or `substreams.clickhouse.yaml`) file:

```yaml
    dbt_config:
      enabled: true
      files: "./dbt"
      run_interval_seconds: 180
```

* Start the `substreams alpha service serve` command in another window (if it is not still running from before...)

```bash
SUBSTREAMS_API_TOKEN=(...) substreams alpha service serve
```

* Stop the previous deployment completely (only a single deployment can run at the same time on local Docker environment)

```bash
substreams alpha service stop
```

* Test your deployment locally, in production mode:

```bash
substreams alpha service deploy substreams.sql.yaml --prod # or substreams.clickhouse.yaml
```

{% hint style="success" %}
**Tip:** If using Clickhouse, you will need to set `sink.config.rest_frontend.enabled` to `true`: it is currently the only way to consume data in a 'production' deployment.
{% endhint %}

* See that the database starts correctly and that the tables defined in `dbt` are being created correctly

* When you are happy with the results, verify or bump the `version` field in `substreams.sql.yaml`, you can generate the `cryptopunks-v0.1.0.spkg` file.

```bash
substreams pack substreams.sql.yaml # or substreams.clickhouse.yaml
```

## Deploy your production package to the "hosted prod" environment

```bash
substreams alpha service deploy cryptopunks-v0.1.0.spkg -e https://deploy.streamingfast.io --prod
```

The production environment does not allow direct SQL access at the moment, so your apps will need to access the data to either the `postgraphile` frontend (or the `rest` frontend when using Clickhouse)
