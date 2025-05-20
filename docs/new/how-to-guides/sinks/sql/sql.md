The **Substreams:SQL service** allows you to consume the data extracted from the blockchain through a SQL database.

<figure><img src="../../../.gitbook/assets/consume/service-sql.png" width="100%" /></figure>

## Requirements

Before you begin, make sure you have:

- A Substreams package (for example, a package that indexes ERC20 tokens).
- A SQL database: Postgres or ClickHouse.
- The [substreams-sink-sql](https://github.com/streamingfast/substreams-sink-sql) CLI installed in your computer.

## Mapping Substreams to SQL

The core function of the SQL sink is to translate your Substreams output (Protobuf data) into SQL tables. You can choose one of three methods depending on your needs:

- [Direct Mapping from Protobuf (Without Annotations)](./protobuf-without-annotations.md)
    * Simplest method.
    * Automatically maps your Protobuf output to SQL tables.
    * Insert-only — no relationships or constraints.
- Direct Mapping from Protobuf (With Annotations)
    * Enables foreign key relationships in your SQL schema.
    * Requires adding annotations to your Protobuf messages (e.g., primary and foreign keys).
    * Still insert-only, but supports relational integrity.
- [`db_out` module](./db_out.md)
    * Gives you full control over the output.
    * Supports insert, update, and upsert operations.
    * Ideal for advanced use cases with evolving or mutable data.

|                               | Without annotations | With annotations | `db_out` module |
|-------------------------------|---------------------|------------------|-----------------|
| SQL relationships             | No                  | Yes              | No              |
| Direct Protobuf<>SQL mappings | Yes                 | No               | No              |
| `INSERT` supported            | Yes                 | Yes              | Yes             |
| `UPDATE` supported            | No                  | No               | Yes             |
| `UPSERT` supported            | No                  | No               | Yes             |


## Installation

Regardless of the ways you choose to map the data, you will have to install the `substreams-sink-sql` CLI in your computer.

### Installing the Binary

1. Download the correct binary, depending on your operating system, from the [substreams-sink-sql GitHub releases](https://github.com/streamingfast/substreams-sink-sql/releases) page.
2. Move the binary to your `$PATH`.

### Installing from Source

1. Clone the [substreams-sink-sql GitHub repository](https://github.com/streamingfast/substreams-sink-sql).
2. Move to the `cmd/substreams-sink-sql` folder.
3. Install the binary using Go

```bash
go build
```

4. Move the binary to your `$PATH`.

