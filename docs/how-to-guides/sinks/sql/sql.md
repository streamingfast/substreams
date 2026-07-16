The **Substreams:SQL Sink** allows you to consume the data extracted from the blockchain through a SQL database.

## Requirements

Before you begin, make sure you have:

- A Substreams package (for example, a package that indexes ERC20 tokens).
- A SQL database: Postgres or ClickHouse.
- The [Substreams CLI](../../cli/installing-the-cli.md) installed on your computer. The SQL sink is built into the `substreams` CLI (`substreams sink postgres` / `substreams sink clickhouse`).

## Mapping Substreams to SQL

The core function of the SQL sink is to translate your Substreams output (Protobuf data) into SQL tables. Choose one of the following methods depending on your needs:

- [Using Relational Mappings "from-proto"](https://github.com/streamingfast/substreams/blob/develop/sink/sql/FROM_PROTO.md)
    * Enables foreign key relationships in your SQL schema.
    * Requires adding annotations to your Protobuf messages (e.g., primary and foreign keys).
    * Currently insert-only.
- [Using Database Changes](./db_out.md)
    * Gives you full control over the output.
    * Supports insert, update, and upsert operations.
    * Ideal for advanced use cases with evolving or mutable data.
    * **NOTE:** In ClickHouse, reorgs are currently supported with delay.

|                               | Relational Mappings | `db_out` module |
|-------------------------------|---------------------|-----------------|
| SQL relationships             | Yes                 | No              |
| Direct Protobuf<>SQL mappings | Yes                 | No              |
| `INSERT` supported            | Yes                 | Yes             |
| `UPDATE` supported            | No                  | Yes             |
| `UPSERT` supported            | No                  | Yes             |


## Installation

The SQL sink is included in the [Substreams CLI](../../cli/installing-the-cli.md) — there is no separate binary to install. Once `substreams` is installed, the `substreams sink postgres` and `substreams sink clickhouse` commands are available.

