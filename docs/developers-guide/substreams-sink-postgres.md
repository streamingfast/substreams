---
description: StreamingFast Substreams PostgreSQL sink
---

# `substreams-sink-postgres` introduction

### Purpose

Learn how to use the StreamingFast [`substreams-sink-postgres`](https://github.com/streamingfast/substreams-sink-postgres) tool with this documentation. A basic Substreams module example is provided to help you get started. We are going to showcase a Substreams module to extract data from the Ethereum blockchain and route it into a protobuf for persistence in a PostgreSQL database.

## Installation

### 1. Install `substreams-sink-postgres`

Install `substreams-sink-postgres` by using the pre-built binary release [available in the official GitHub repository](https://github.com/streamingfast/substreams-sink-postgres/releases).

Extract `substreams-sink-postgres` into a folder and ensure this folder is referenced globally via your `PATH` environment variable.

### 2. Set up accompanying code example

Access the accompanying code example for this tutorial in the official `substreams-sink-postgres` repository. You will find the Substreams project for the tutorial in the `docs/tutorial/` directory.

To create the required protobuf files, run the included make codegen command.

```bash
make codegen
```

To ensure proper setup and functionality, use your installation of the [`substreams` CLI](https://substreams.streamingfast.io/reference-and-specs/command-line-interface) to run the example code.

Use the `make build` and `substreams run` commands to verify the setup for the example project.

Use the included `make` command to build the Substreams module.

```bash
make
```

Use the `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command to run the project.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml db_out --start-block 100 --stop-block +1
```

When you use the `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command, you will see output that looks similar to the following:

```bash
Connected - Progress messages received: 0 (0/sec)
Connected - Progress messages received: 101 (0/sec)
Backprocessing history up to requested target block 100:
(hit 'm' to switch mode)

store_block_meta_start            0  ::  0-99
----------- NEW BLOCK #100 (100) ---------------
all done
```

### Module handler for sink

The Rust source code file [`lib.rs`](#) contains an example code, the `db_out` module handler, which prepares and returns the module's [`DatabaseChanges`](https://docs.rs/substreams-database-change/latest/substreams_database_change/pb/database/struct.DatabaseChanges.html) output. The `substreams-sink-postgres` tool captures the data sent out of the Substreams module and routes it into the appropriate columns and tables in the PostgreSQL database.

```rust
#[substreams::handlers::map]
fn db_out(block_meta_start: store::Deltas<DeltaProto<BlockMeta>>) -> Result<DatabaseChanges, substreams::errors::Error> {
    let mut database_changes: DatabaseChanges = Default::default();
    transform_block_meta_to_database_changes(&mut database_changes, block_meta_start);
    Ok(database_changes)
}
```

To gain a full understanding of the procedures and steps required for a database sink Substreams module, review the code in [`lib.rs`](#). The complete code includes the addition of a Substreams store module and other helper functions related to the database.

**DatabaseChanges**

The [`DatabaseChanges`](https://github.com/streamingfast/substreams-database-change/blob/develop/proto/database/v1/database.proto) protobuf definition can be viewed at the following link for a peek into the crates implementation.

https://github.com/streamingfast/substreams-database-change/blob/develop/proto/database/v1/database.proto

Full source code is provided by StreamingFast for the [`DatabaseChanges`](https://github.com/streamingfast/substreams-database-change) crate found in its official GitHub repository.

https://github.com/streamingfast/substreams-database-change

**Note**: An output type of `proto:substreams.database.v1.DatabaseChanges` is required by the map module in the Substreams manifest when working with a sink.

## 3. Install PostgreSQL

To proceed with this tutorial, you must have a working PostgreSQL installation. Obtain the software by [downloading it from the vendor](https://www.postgresql.org/download/) and [install it by following the instructions](https://www.postgresql.org/docs/current/tutorial-install.html) for your operating system and platform.

If you encounter any issues, [refer to the Troubleshooting Installation page](https://wiki.postgresql.org/wiki/Troubleshooting_Installation) on the official PostgreSQL Wiki for assistance.

**DEV NOTE**: Explain Docker install too?

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

## 5. Run command for tool and schema

After creating the database in step four, you must set it up using the schema provided in the tutorial.

Use the following command to run the `substreams-sink-postgres` tool and set up the database for the tutorial.

```bash
substreams-sink-postgres setup "psql://postgres:pass1@localhost/substreams_example?sslmode=disable" schema.sql
```

## 6. Sink data to PostgreSQL

The `substreams-sink-postgres` tool sinks data from the Substreams module to the PostgreSQL database. Use the tool's `run` command, followed by the connection string, endpoint, manifest file, and module map name, to execute the tool.

The connection string requires the database IP address, username, and password, which depend on your PostgreSQL installation.

You may need to set a password for the default `postgres` database user account by using the command: `ALTER USER postgres PASSWORD 'somepasswordhere';`

To prevent the following error, ensure the connection string includes `?sslmode=disable` at the end.

```bash
load psql table: retrieving table and schema: pq: SSL is not enabled on the server
```

The endpoint needs to match the blockchain targeted in the Substreams module. The example Substreams module uses the Ethereum blockchain.

The manifest needs to match the filename used in the Substreams module. As seen in the example, the prescribed naming convention from StreamingFast uses the filename `substreams.yaml`.

The name of the example module passed in the command to the `substreams-sink-postgres` tool is `db_out`.

```bash
substreams-sink-postgres run \ "psql://<username>:<password>@<database_ip_address>/substreams_example?sslmode=disable" \ "mainnet.eth.streamingfast.io:443" \ "substreams.yaml" \ db_out
```

Successful output from the `substreams-sink-postgres` tool will resemble the following:

```bash
2023-01-18T12:32:19.107-0800 INFO (sink-postgres) starting prometheus metrics server {"listen_addr": "localhost:9102"}
2023-01-18T12:32:19.107-0800 INFO (sink-postgres) sink from psql {"dsn": "psql://postgres:pass1@localhost/substreams_example?sslmode=disable", "endpoint": "mainnet.eth.streamingfast.io:443", "manifest_path": "substreams.yaml", "output_module_name": "db_out", "block_range": ""}
2023-01-18T12:32:19.107-0800 INFO (sink-postgres) starting pprof server {"listen_addr": "localhost:6060"}
2023-01-18T12:32:19.127-0800 INFO (sink-postgres) reading substreams manifest {"manifest_path": "substreams.yaml"}
2023-01-18T12:32:20.283-0800 INFO (pipeline) computed start block {"module_name": "store_block_meta_start", "start_block": 0}
2023-01-18T12:32:20.283-0800 INFO (pipeline) computed start block {"module_name": "db_out", "start_block": 0}
2023-01-18T12:32:20.283-0800 INFO (sink-postgres) validating output store {"output_store": "db_out"}
2023-01-18T12:32:20.285-0800 INFO (sink-postgres) resolved block range {"start_block": 0, "stop_block": 0}
2023-01-18T12:32:20.287-0800 INFO (sink-postgres) ready, waiting for signal to quit
2023-01-18T12:32:20.287-0800 INFO (sink-postgres) starting stats service {"runs_each": "2s"}
2023-01-18T12:32:20.288-0800 INFO (sink-postgres) no block data buffer provided. since undo steps are possible, using default buffer size {"size": 12}
2023-01-18T12:32:20.288-0800 INFO (sink-postgres) starting stats service {"runs_each": "2s"}
2023-01-18T12:32:20.730-0800 INFO (sink-postgres) session init {"trace_id": "4605d4adbab0831c7505265a0366744c"}
2023-01-18T12:32:21.041-0800 INFO (sink-postgres) flushing table entries {"table_name": "block_data", "entry_count": 2}
2023-01-18T12:32:21.206-0800 INFO (sink-postgres) flushing table entries {"table_name": "block_data", "entry_count": 2}
2023-01-18T12:32:21.319-0800 INFO (sink-postgres) flushing table entries {"table_name": "block_data", "entry_count": 0}
2023-01-18T12:32:21.418-0800 INFO (sink-postgres) flushing table entries {"table_name": "block_data", "entry_count": 0}
```

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

You will find that the cursor is saved in the cursors table of the `substreams_example` database.

**TODO**: Discussion about where Substreams cursor is saved (in a table) -- I need additional input here on what exactly we want to convey to the reader. I understand there's another table named cursors, but how is this used and what exactly does the dev/reader need to know?

### Batching

- Discussion about batching of writes (each 1000 blocks when not live, still need to be determined when we will be live) -- I need additional input here. Is this something that's functional enough to include in this documentation at this time?

## Conclusion and review

Routing data extracted from the blockchain using Substreams is a powerful and useful feature. With Substreams, you can route data to various types of sinks, including files and databases such as PostgreSQL. For more information on other types of sinks and sinking strategies, consult the core Substreams sinks documentation at https://substreams.streamingfast.io/developers-guide/substreams-sinks.

The StreamingFast `substreams-sink-postgres` tool allows developers to route data extracted from a blockchain to a PostgreSQL database. To route data to PostgreSQL using Substreams, you must install the `substreams-sink-postgres` tool, create or clone the example Substreams module, install PostgreSQL, create the example database, import the schema through the `substreams-sink-postgres` tool, and then begin the sinking process by running the `run` command. Once the data is in the `substreams_example` database, you can use standard PostgreSQL tooling and SQL language to view it.

---

<b>DEV NOTES</b>

- Do we want to move the code creating the queries into a db.rs file similar to how it is in the eth-meta example? It's a bit of a tough call. Ideally, we want to show them it in this same file because they'll need to edit it to work with the data they want to extract and persist to the db. Having it in another file contributes to context switching and additional abstraction and cognitive load.
