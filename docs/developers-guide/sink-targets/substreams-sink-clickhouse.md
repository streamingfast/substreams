---
description: Aleno Substreams ClickHouse sink
---

# [`Substreams`](https://substreams.streamingfast.io/) [Clickhouse](https://clickhouse.com/) sink module


> [`substreams-sink-clickhouse`](https://github.com/aleno-ai/substreams-sink-clickhouse) is a tool that allows developers to pipe data extracted metrics from a blockchain into a ClickHouse DBMS for warehousing purposes.

> This sink is very similar to the [substreams-sink-postgres](https://github.com/streamingfast/substreams-sink-postgres) and one could refer to its [docs](./substreams-sink-postgres.md) to understand its concepts.


### Quickstart

1. Install `substreams-sink-clickhouse` by using the pre-built binary release [available in the releases page](https://github.com/aleno-ai/substreams-sink-clickhouse/releases). Extract `substreams-sink-clickhouse` binary into a folder and ensure this folder is referenced globally via your `PATH` environment variable.

    > **Note** Or install from source directly `go install github.com/aleno-ai/substreams-sink-clickhouse/cmd/substreams-sink-clickhouse@latest`.

1. Start Docker Compose:

    ```bash
    docker compose up
    ```

    > **Note** Feel free to skip this step if you already have a running ClickHouse instance accessible, don't forget to update the connection string in the command below.

2. Setup ClickHouse

    Connect to ClickHouse

    ```bash
    docker compose exec ch_server clickhouse-client -u dev-node --password insecure-change-me-in-prod -h localhost
    ```

    And create necessary tables to run the sink

    ```sql
        CREATE TABLE block_meta
    (
        id          String,
        at          String,
        number      Int32,
        hash        String,
        parent_hash String,
        timestamp   String,
        PRIMARY KEY (id),
    )
    ENGINE = MergeTree()
    ORDER BY id;

    CREATE TABLE cursors
    (
        id         String,
        cursor     String,
        block_num  Int64,
        block_id   String,
        PRIMARY KEY (id)
    ) ENGINE = MergeTree()
    ORDER BY id;
    ```

    > **Note**: Each create table query must be run independently as ClickHouse doesn't support multiple create table queries at once.

3. Run the sink

    Use the precompiled Ethereum Block Meta [substreams](https://github.com/streamingfast/substreams-eth-block-meta/releases/latest)

    > **Note**: To connect to Substreams you will need an authentication token, follow this [guide](https://substreams.streamingfast.io/reference-and-specs/authentication) to obtain one.

    ```shell
    substreams-sink-clickhouse run \
        "clickhouse://dev-node:insecure-change-me-in-prod@localhost:8123" \
        "mainnet.eth.streamingfast.io:443" \
        https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.4.3/substreams-eth-block-meta-v0.4.3.spkg \
        db_out
    ```

### Output Module

To be accepted by `substreams-sink-clickhouse`, your module output's type must be a [sf.substreams.sink.database.v1.DatabaseChanges](https://github.com/streamingfast/substreams-sink-database-changes/blob/develop/proto/sf/substreams/sink/database/v1/database.proto#L7) message. The Rust crate [substreams-database-change](https://docs.rs/substreams-database-change/latest/substreams_database_change) contains bindings and helpers to implement it easily. Some project implementing `db_out` module for reference:
- [substreams-eth-block-meta](https://github.com/streamingfast/substreams-eth-block-meta/blob/master/src/lib.rs#L35) (some helpers found in [db_out.rs](https://github.com/streamingfast/substreams-eth-block-meta/blob/master/src/db_out.rs#L6))

By convention, we name the `map` module that emits [sf.substreams.sink.database.v1.DatabaseChanges](https://github.com/streamingfast/substreams-sink-database-changes/blob/develop/proto/sf/substreams/sink/database/v1/database.proto#L7) output `db_out`.

### ClickHouse DSN

The connection string is provided using a simple string format respecting the URL specification. The DSN format is:

```sh
clickhouse://<user>:<password>@<host>:<port>/<dbname>[?<options>]
```

Where `<options>` is URL query parameters in `<key>=<value>` format, multiple options are separated by & signs. Supported options can be seen on libpq official documentation. The options `<user>`, `<password>`, `<host>`, `<port>` and `<dbname>` should not be passed in `<options>` as they are automatically extracted from the DSN URL.

