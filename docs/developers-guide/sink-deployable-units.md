---
description: Working with Deployable Units (alpha feature)
---

# Defining a deployable unit

## Overview

Deployable units allow you to define everything that is needed to run a substreams with a sink, inside the package itself.

{% hint style="info" %}
**Note**: Currently, only SQL sink supports this feature.
{% endhint %}

## Requirements

* A substreams that can output to a sink (follow [`substreams-sink-sql`](https://substreams.streamingfast.io/developers-guide/sink-targets/substreams-sink-sql.md))
* Substreams CLI version v1.1.15 or above (https://github.com/streamingfast/substreams/releases)
* To run the development environmentk, you will need :
  * [Docker](https://docs.docker.com/engine/install/) to be installed
  * An [authentication token](https://substreams.streamingfast.io/reference-and-specs/authentication)

## Adding sink information to your substreams manifest

* Add the following to your substreams.yaml manifest:

```yaml
imports:
  sql: https://github.com/streamingfast/substreams-sink-sql/releases/download/protodefs-v1.0.3/substreams-sink-sql-protodefs-v1.0.3.spkg

network: mainnet

sink:
  module: db_out
  type: sf.substreams.sink.sql.v1.Service
  config:
    schema: "./schema.sql"
    postgraphile_frontend:
      enabled: true
```

Explanation:
  * The `imports.sql` field embeds the protobuf definition for `sf.substreams.sink.sql.v1.Service`
  * The `network` field is a string that represents the chain. While there is no official canonical identifer for each network, this is a good reference (see CLI Name column) https://thegraph.com/docs/en/developing/supported-networks/
  * The `sink.module` is simply the name of the module that outputs to the sink
  * The `sink.type` will be the fully-qualified domain name of the protobuf message supported by the sink
  * The `sink.config` should map with the protobuf message type specified under `sink.type`, see (sf.substreams.sink.sql.v1 protobuf definition)[https://github.com/streamingfast/substreams-sink-sql/blob/develop/proto/sf/substreams/sink/sql/v1/services.proto]

* Run `substreams info substreams.yaml` to validate that your changes are correctly decoded, you should see something like this:
```
Sink config:
----
type: sf.substreams.sink.sql.v1.Service
configs:
- schema: (371 bytes) MD5SUM: 280ada56ad9f83f58891872bf93e4794 [LOADED_FILE]
- postgraphile_frontend:
  - enabled: true
(...)
```

# Running the stack on a local dev environment

1. Set your authentication token in your environment: `export SUBSTREAMS_API_TOKEN="your-token"`
1. Run the `serve` command in a shell (this is the development server that will create docker containers to run the sink and database) `substreams alpha sink-serve` (it will store data under './sink-data' by default, override to your preference with `--data-dir`)
1. From another shell, deploy your Substreams deployable unit: `substreams alpha sink-deploy ./substreams.yaml` and see the output services details:
```
Deploying... (creating services, please wait)
Deployed substreams sink "1ef89c74":
  Status: RUNNING ()
Services:
  - 1ef89c74-pgweb: PGWeb service "1ef89c74-pgweb" available at URL: 'http://localhost:8081'
  - 1ef89c74-postgraphile: Postgraphile service "1ef89c74-postgraphile" available at URL: 'http://localhost:3000/graphiql' (API at 'http://localhost:3000/graphql')
  - 1ef89c74-postgres: PostgreSQL service "1ef89c74-postgres" available at DSN: 'postgres://dev-node:insecure-change-me-in-prod@localhost:5432/substreams?sslmode=disable'
  - 1ef89c74-sink: Sink service (no exposed port). Use 'substreams alpha sink-info 1ef89c74-sink' to see last processed block or 'docker logs 1ef89c74-sink' to see the logs.
```

1. You can explore the different services directly from your browser:
  * Postgraphile: http://localhost:3000/graphiql
  * PGWeb: http://localhost:8081/
1. After a few seconds, the command `substreams alpha sink-info` should give you information about the progress of the sink (ex: `Last processed block: 11000`)
1. You can pause the sink process and keep the database and tools available by running `substreams alpha sink-pause`
1. Resume the sink process by running `substreams alpha sink-resume`
1. To deploy a new version of your substreams to your development environment, simply use `substreams alpha sink-update ./substreams.yaml` with the new code. The sql-sink will continue feeding from where it left off, unless you use `--reset`, forcing the sink to start from the beginning.
1. After deploying a new version, you can check the "output module hash" from the `sink-info` command and see confirm that it matches the module hash from your `substreams info` command. (ex: `Output module: db_out (dec326aecb9e27fbfb67d1748a91f7f84746ec27)`). The version number from your substreams.yaml is also displayed as part of the sink-info output.
1. When you're done, use `substreams alpha sink-stop` or simply hit "ctrl-c" on the terminal running `sink-serve` and let it shut down the docker containers completely. You can always use `docker ps` to list all running containers on your machine.
* Get the full list of sink management commands by running `substremas alpha help`

{% hint style="info" %}
**Note**: The "sink" will keep running until it is stopped, either by hitting "ctrl-c" on the `substreams alpha sink-serve` terminal, or by using the commands `sink-pause`, `sink-stop` or `sink-remove`.
{% endhint %}
