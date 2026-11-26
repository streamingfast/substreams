The **SQL** sink writes the data extracted from the blockchain into a PostgreSQL or ClickHouse database. It is built into the `substreams` CLI as `substreams sink postgres` and `substreams sink clickhouse`.

## Requirements

Before you begin, make sure you have:

- A Substreams package (for example, a package that indexes ERC20 tokens).
- A SQL database: Postgres or ClickHouse.
- The [Substreams CLI](../../cli/installing-the-cli.md) installed on your computer. The SQL sink is built into the `substreams` CLI (`substreams sink postgres` / `substreams sink clickhouse`).

## Mapping Substreams to SQL

The core function of the SQL sink is to translate your Substreams output (Protobuf data) into SQL tables. Choose one of the following methods depending on your needs:

- [Using Relational Mappings Mode](./relational-mappings.md)
    * Enables foreign key relationships in your SQL schema.
    * Requires adding annotations to your Protobuf messages (e.g., primary and foreign keys).
    * Currently insert-only.
- [Using Database Changes Mode](./db_out.md)
    * Gives you full control over the output.
    * Supports insert, update, and upsert operations.
    * Ideal for advanced use cases with evolving or mutable data.
    * **NOTE:** In ClickHouse, reorgs are currently supported with delay.

|                               | Relational Mappings Mode | Database Changes Mode (`db_out`) |
|-------------------------------|--------------------------|----------------------------------|
| SQL relationships             | Yes                      | No                               |
| Direct Protobuf<>SQL mappings | Yes                      | No                               |
| `INSERT` supported            | Yes                      | Yes                              |
| `UPDATE` supported            | No                       | Yes                              |
| `UPSERT` supported            | No                       | Yes                              |


## Installation

The SQL sink is included in the [Substreams CLI](../../cli/installing-the-cli.md). There is no separate binary to install. If you used the standalone `substreams-sink-sql` binary, see [Migrating from substreams-sink-sql](./migration.md).

Both commands detect the mode from the output module type: a `DatabaseChanges` output runs in Database Changes Mode, any other output runs in Relational Mappings Mode. The database is given with `--dsn` or the `SUBSTREAMS_SINK_DSN` environment variable. See the [DSN Reference](../../../references/sql/dsn-reference.md).

## PostgreSQL

Start a local database:

```bash
docker run --name postgres-db -e POSTGRES_PASSWORD=password -e POSTGRES_DB=substreams -p 5432:5432 -d postgres:15
export DSN="postgres://postgres:password@localhost:5432/substreams?sslmode=disable"
```

Create the tables, then run the sink:

```bash
substreams sink postgres setup ./substreams.yaml --dsn "$DSN"
substreams sink postgres ./substreams.yaml --dsn "$DSN"
```

`substreams sink postgres` also has `generate-csv` and `inject-csv` to backfill a large range faster in Database Changes Mode, `constraints` to apply foreign keys after a backfill, and `tools` to read or change the stored cursor. Run `substreams sink postgres --help` for details.

## ClickHouse

Start a local database:

```bash
docker run --name clickhouse-db -e CLICKHOUSE_PASSWORD=password -p 9000:9000 -p 8123:8123 -d clickhouse/clickhouse-server
export DSN="clickhouse://default@localhost:9000/default"
export CLICKHOUSE_PASSWORD=password
```

When the DSN has no password, the sink reads it from `CLICKHOUSE_PASSWORD` (change it with `--password-envvar`). Create the tables, then run the sink:

```bash
substreams sink clickhouse setup ./substreams.yaml --dsn "$DSN"
substreams sink clickhouse ./substreams.yaml --dsn "$DSN"
```

For ClickHouse Cloud, use port `9440` and `secure=true`. In Database Changes Mode, reorgs are handled with a delay. See [Reorg Handling](../../../references/sql/reorg-handling.md). Run `substreams sink clickhouse --help` for the cluster and table options.
