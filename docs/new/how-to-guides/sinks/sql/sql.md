The **Substreams:SQL service** allows you to consume the data extracted from the blockchain through a SQL database.

<figure><img src="../../../.gitbook/assets/consume/service-sql.png" width="100%" /></figure>

## Requirements

Before you begin, make sure you have:

- A Substreams package (for example, a package that indexes ERC20 tokens).
- A SQL database: Postgres or ClickHouse.
- The [substreams-sink-sql](https://github.com/streamingfast/substreams-sink-sql) CLI installed in your computer.

## Mapping Substreams to SQL

The core function of the SQL sink is to translate your Substreams output (Protobuf data) into SQL tables. Choose one of the following methods depending on your needs:

- [Relational Mappings](./relational-mappings.md)
    * Enables foreign key relationships in your SQL schema.
    * Requires adding annotations to your Protobuf messages (e.g., primary and foreign keys).
    * Currently insert-only.
- [`db_out` module](./db_out.md)
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

Enabling `substreams-sink-sql` in your [Substreams CLI](../../../../references/cli/installing-the-cli.md).

1. Download the current binary, optionally depending on your operating system, from the [substreams-sink-sql GitHub releases](https://github.com/streamingfast/substreams-sink-sql/releases) page.
1. Move the binary to your `$PATH`.

### Installing from Source

1. Clone the [substreams-sink-sql GitHub repository](https://github.com/streamingfast/substreams-sink-sql).
1. Install the binary using Go.

```bash
go install ./cmd/substreams-sink-sql
```

1. Make sure GO is installed and the Go bin directory is in your $PATH.

