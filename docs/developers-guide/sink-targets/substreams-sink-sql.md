---
description: StreamingFast Substreams SQL sink
---

# `substreams-sink-sql` introduction

### Purpose

Learn how to use the StreamingFast [`substreams-sink-sql`](https://github.com/streamingfast/substreams-sink-sql) tool with this documentation. A basic Substreams module example is provided to help you get started. We are going to showcase a Substreams module to extract data from the Ethereum blockchain and route it into a Protobuf for persistence in a SQL database.

The `substreams-sink-sql` today supports two database drivers namely _PostgresSQL_ and _Clickhouse_. The tutorial below will focus on Postgres but we will describe how to connect to the other supported drivers.

## Installation

### 1. Install `substreams-sink-sql`

Install `substreams-sink-sql` by using the pre-built binary release [available in the official GitHub repository](https://github.com/streamingfast/substreams-sink-sql/releases).

Extract `substreams-sink-sql` into a folder and ensure this folder is referenced globally via your `PATH` environment variable.

### 2. Set up accompanying code example

Access the accompanying code example for this tutorial in the official `substreams-sink-sql` repository. You will find the Substreams project for the tutorial in the [docs/tutorial](https://github.com/streamingfast/substreams-sink-sql/tree/develop/docs/tutorial) directory.

To create the required Protobuf files, run the included `make protogen` command.

```bash
make protogen
```

To ensure proper setup and functionality, use your installation of the [`substreams` CLI](https://substreams.streamingfast.io/reference-and-specs/command-line-interface) to run the example code.

Use the `make build` and `make stream_db` commands to verify the setup for the example project. Use the included `make` command to build the Substreams module.

```bash
make build
make stream_db
```

### Module handler for sink

The Rust source code file [`lib.rs`](https://github.com/streamingfast/substreams-sink-sql/blob/develop/docs/tutorial/src/lib.rs) contains an example code, the `db_out` module handler, which prepares and returns the module's [`DatabaseChanges`](https://docs.rs/substreams-database-change/latest/substreams_database_change/pb/database/struct.DatabaseChanges.html) output. The `substreams-sink-sql` tool captures the data sent out of the Substreams module and routes it into the appropriate columns and tables in the SQL database.

```rust
#[substreams::handlers::map]
fn db_out(block_meta_start: store::Deltas<DeltaProto<BlockMeta>>) -> Result<DatabaseChanges, substreams::errors::Error> {
    let mut database_changes: DatabaseChanges = Default::default();
    transform_block_meta_to_database_changes(&mut database_changes, block_meta_start);
    Ok(database_changes)
}
```

To gain a full understanding of the procedures and steps required for a database sink Substreams module, review the code in [`lib.rs`](https://github.com/streamingfast/substreams-sink-sql/blob/develop/docs/tutorial/src/lib.rs). The complete code includes the addition of a Substreams store module and other helper functions related to the database.

**DatabaseChanges**

The [`DatabaseChanges`](https://github.com/streamingfast/substreams-sink-database-changes/blob/develop/proto/sf/substreams/sink/database/v1/database.proto#L7) Protobuf definition can be viewed at the following link for a peek into the crates implementation.

When developing your Substreams, the Rust crate [substreams-database-change](https://docs.rs/substreams-database-change/latest/substreams_database_change) can be used to create the required `DatabaseChanges` output type.

**Note**: An output type of `proto:sf.substreams.sink.database.v1.DatabaseChanges` is required by the map module in the Substreams manifest when working with this sink.

## 3. Install PostgreSQL

To proceed with this tutorial, you must have a working PostgreSQL installation. Obtain the software by [downloading it from the vendor](https://www.postgresql.org/download/) and [install it by following the instructions](https://www.postgresql.org/docs/current/tutorial-install.html) for your operating system and platform.

If you encounter any issues, [refer to the Troubleshooting Installation page](https://wiki.postgresql.org/wiki/Troubleshooting_Installation) on the official PostgreSQL Wiki for assistance.

## 4. Create example database

To store the blockchain data output by the Substreams module, you must create a new database in your PostgreSQL installation. The tutorial provides a schema and the PostgreSQL sink tool that handle the detailed aspects of the database design.

Use the `psql` command in your terminal to launch PostgreSQL.

Upon successful launch, you will see a prompt similar to the following, ready to accept commands for PostgreSQL.

```bash
psql (15.1)
Type "help" for help.

default-database-name=#
```

Use the following `SQL` command to create the example database:

```bash
CREATE DATABASE "substreams_example";
```

## 5. Create configuration file

Once the database has been created, you must now define the Substreams Sink Config in a Substreams manifest creating a deployable unit.

Let's create a folder `sink` and in it create a file called `substreams.dev.yaml` with the following content:

```yaml
specVersion: v0.1.0
package:
  name: "<name>"
  version: <version>

imports:
  sql: https://github.com/streamingfast/substreams-sink-sql/releases/download/protodefs-v1.0.1/substreams-sink-sql-protodefs-v1.0.1.spkg
  main: ../substreams.yaml

network: 'mainnet'

sink:
  module: main:db_out
  type: sf.substreams.sink.sql.v1.Service
  config:
    schema: "../schema.sql"
```

The `package.name` and `package.version` are meant to be replaced to fit your project.

The `imports.main` defines your Substreams manifest that you want to sink. The `sink.module` defines which import key (`main` here) and which module's name (`db_out` here).

The `network` field defines which network this deployment should be part of, in our case `mainnet`

The `sink.type` defines the type of the config that we are expecting, in our case it's [sf.substreams.sink.sql.v1.Service](https://buf.build/streamingfast/substreams-sink-sql/docs/main:sf.substreams.sink.sql.v1#sf.substreams.sink.sql.v1.Service) (click on the link to see the message definition).

The `sink.config` is the instantiation of this `sink.type` with the config fully filled. Some config are special because they load from a file or from a folder. For example in our case the `sink.config.schema` is defined with a Protobuf option `load_from_file` which means the content of the `../schema.sql` will actually be inlined in the Substreams manifest.

> The final final can be found at [`sink/substreams.dev.yaml`](https://github.com/streamingfast/substreams-sink-sql/blob/develop/docs/tutorial/sink/substreams.dev.yaml)

## 6. Run setup command

Use the following command to run the `substreams-sink-sql` tool and set up the database for the tutorial.

```bash
substreams-sink-sql setup "psql://dev-node:insecure-change-me-in-prod@127.0.0.1:5432/substreams_example?sslmode=disable" ./sink/substreams.dev.yaml
```

The `"psql://..."` is the DSN (Database Source Name) containing the connection details to your database packed as an URL. The `scheme` (`psql` here) part of the DSN's url defines which driver to use, `psql` is what we are going to use here, see [Drivers](#drivers) section below to see what other DSN you can use here.

The DSN's URL defines the database IP address, username, and password, which depend on your PostgreSQL installation. Adjust `dev-node` to your own username `insecure-change-me-in-prod` to your password and `127.0.0.1:5432` to where your database can be reached.

### Drivers

For DSN configuration for the currently supported drivers, see the list below:

- [Clickhouse DSN](https://github.com/streamingfast/substreams-sink-sql#clickhouse)
- [PostgresSQL DSN](https://github.com/streamingfast/substreams-sink-sql#postgresql)

## 7. Sink data to database

The `substreams-sink-sql` tool sinks data from the Substreams module to the SQL database. Use the tool's `run` command, followed by the endpoint to reach and your Substreams config file to use:

```bash
substreams-sink-sql run "psql://dev-node:insecure-change-me-in-prod@127.0.0.1:5432/substreams_example?sslmode=disable" ./sink/substreams.dev.yaml
```

The endpoint needs to match the blockchain targeted in the Substreams module. The example Substreams module uses the Ethereum blockchain.

Successful output from the `substreams-sink-sql` tool will resemble the following:

```log
2023-01-18T12:32:19.107-0800 INFO (sink-sql) starting prometheus metrics server {"listen_addr": "localhost:9102"}
2023-01-18T12:32:19.107-0800 INFO (sink-sql) sink from psql {"dsn": "psql://dev-node:insecure-change-me-in-prod@127.0.0.1:5432/substreams_example?sslmode=disable", "endpoint": "mainnet.eth.streamingfast.io:443", "manifest_path": "substreams.yaml", "output_module_name": "db_out", "block_range": ""}
2023-01-18T12:32:19.107-0800 INFO (sink-sql) starting pprof server {"listen_addr": "localhost:6060"}
2023-01-18T12:32:19.127-0800 INFO (sink-sql) reading substreams manifest {"manifest_path": "sink/substreams.dev.yaml"}
2023-01-18T12:32:20.283-0800 INFO (pipeline) computed start block {"module_name": "store_block_meta_start", "start_block": 0}
2023-01-18T12:32:20.283-0800 INFO (pipeline) computed start block {"module_name": "db_out", "start_block": 0}
2023-01-18T12:32:20.283-0800 INFO (sink-sql) validating output store {"output_store": "db_out"}
2023-01-18T12:32:20.285-0800 INFO (sink-sql) resolved block range {"start_block": 0, "stop_block": 0}
2023-01-18T12:32:20.287-0800 INFO (sink-sql) ready, waiting for signal to quit
2023-01-18T12:32:20.287-0800 INFO (sink-sql) starting stats service {"runs_each": "2s"}
2023-01-18T12:32:20.288-0800 INFO (sink-sql) no block data buffer provided. since undo steps are possible, using default buffer size {"size": 12}
2023-01-18T12:32:20.288-0800 INFO (sink-sql) starting stats service {"runs_each": "2s"}
2023-01-18T12:32:20.730-0800 INFO (sink-sql) session init {"trace_id": "4605d4adbab0831c7505265a0366744c"}
2023-01-18T12:32:21.041-0800 INFO (sink-sql) flushing table rows {"table_name": "block_data", "row_count": 2}
2023-01-18T12:32:21.206-0800 INFO (sink-sql) flushing table rows {"table_name": "block_data", "row_count": 2}
2023-01-18T12:32:21.319-0800 INFO (sink-sql) flushing table rows {"table_name": "block_data", "row_count": 0}
2023-01-18T12:32:21.418-0800 INFO (sink-sql) flushing table rows {"table_name": "block_data", "row_count": 0}
```

{% hint style="info" %}
**Note**: If you have an error looking like `load psql table: retrieving table and schema: pq: SSL is not enabled on the server`, it's because SSL is not enabled to reach you database, add `?sslmode=disable` at the end of the `sink.config.dsn` value to connect without SSL.
{% endhint %}

You can view the database structure by using the following command, after launching PostgreSQL through the `psql` command.

```bash
<default_database_name>=# \c substreams_example
```

The table information is displayed in the terminal resembling the following:

```bash
           List of relations
 Schema |    Name    | Type  |  Owner
--------+------------+-------+----------
 public | block_data | table | postgres
 public | cursors    | table | postgres
(2 rows)
```

You can view the data extracted by Substreams and routed into the database table by using the following command:

```bash
substreams_example=# SELECT * FROM "block_data";
```

Output similar to the following is displayed in the terminal:

```bash
         id         | version |         at          | number |                               hash                               |                           parent_hash                            |      timestamp
--------------------+---------+---------------------+--------+------------------------------------------------------------------+------------------------------------------------------------------+----------------------
 day:first:19700101 |         | 1970-01-01 00:00:00 | 0      | d4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3 | 0000000000000000000000000000000000000000000000000000000000000000 | 1970-01-01T00:00:00Z
 month:first:197001 |         | 1970-01-01 00:00:00 | 0      | d4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3 | 0000000000000000000000000000000000000000000000000000000000000000 | 1970-01-01T00:00:00Z
 day:first:20150730 |         | 2015-07-30 00:00:00 | 1      | 88e96d4537bea4d9c05d12549907b32561d3bf31f45aae734cdc119f13406cb6 | d4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3 | 2015-07-30T15:26:28Z
 month:first:201507 |         | 2015-07-01 00:00:00 | 1      | 88e96d4537bea4d9c05d12549907b32561d3bf31f45aae734cdc119f13406cb6 | d4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3 | 2015-07-30T15:26:28Z
 day:first:20150731 |         | 2015-07-31 00:00:00 | 6912   | ab79f822909750f88dfb9dd0350c1ebe98d5495e9c969cdeb6e0ac993b80175b | 8ffd8c04cb89ef45e0e1163639d51d9ed4fa03dd169db90123a1e047361b46fe | 2015-07-31T00:00:01Z
 day:first:20150801 |         | 2015-08-01 00:00:00 | 13775  | 2dcecad4cf2079d18169ca05bc21e7ba0add7132b9382984760f43f2761bd822 | abaabb8f8b7f7fa07668fb38fd5a08da9814cd8ad18a793e54eef6fa9b794ab4 | 2015-08-01T00:00:03Z
 month:first:201508 |         | 2015-08-01 00:00:00 | 13775  | 2dcecad4cf2079d18169ca05bc21e7ba0add7132b9382984760f43f2761bd822 | abaabb8f8b7f7fa07668fb38fd5a08da9814cd8ad18a793e54eef6fa9b794ab4 | 2015-08-01T00:00:03Z
```

### Cursors

When you use Substreams, it sends back a block to a consumer using an opaque cursor. This cursor points to the exact location within the blockchain where the block is. In case your connection terminates or the process restarts, upon re-connection, Substreams sends back the cursor of the last written bundle in the request so that the stream of data can be resumed exactly where it left off and data integrity is maintained.

You will find that the cursor is saved in the `cursors` table of the `substreams_example` database.

### Batching

Insertion for historical blocks is performed in batched to increase ingestion speed. The `--flush-interval` flag can be used to change the default value of 1000 blocks. Also, the flag `--live-block-time-delta <duration>` can be used to change the delta at which we start considering blocks to be live, the logic is `isLive = (now() - block.timestamp) < valueOfFlag(live-block-time-delta)`.

## Conclusion and review

Routing data extracted from the blockchain using Substreams is a powerful and useful feature. With Substreams, you can route data to various types of sinks, including files and databases such as PostgreSQL. For more information on other types of sinks and sinking strategies, consult the core Substreams sinks documentation at https://substreams.streamingfast.io/developers-guide/substreams-sinks.
