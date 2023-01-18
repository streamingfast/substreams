---
description: StreamingFast Substreams PostgreSQL sink
---

# `substreams-sink-postgres` introduction

### Purpose

This documentation exists to assist you in understanding and beginning to use the StreamingFast [`substreams-sink-postgres`](https://github.com/streamingfast/substreams-sink-postgres) tool. The Substreams module paired with this tutorial is a basic example demonstrating how to use Substreams and PostgreSQL together.

### Overview

The [`substreams-sink-postgres`](https://github.com/streamingfast/substreams-sink-postgres) tool provides the ability to pipe data extracted from a blockchain into a PostgreSQL database.

<<<<<<< Updated upstream
=======
### Outline

1. Create/clone Substreams module (and test)
2. Install Postgres (differs per platform)
3. Install Postgres sink tool
4. Launch Postgres in terminal by using the `psql` command. Then create the example database by using: `CREATE DATABASE "substreams_example";`
5. Run command for tool and schema: `substreams-sink-postgres setup "psql://postgres:pass1@localhost/substreams_example?sslmode=disable" schema.sql`
6. Run the tool with the command:

```bash
substreams-sink-postgres run \ "psql://postgres:pass1@localhost/substreams_example?sslmode=disable" \ "mainnet.eth.streamingfast.io:443" \ "substreams.yaml" \ db_out
```

**NOTE:** The default user may need to have a password set by using: `ALTER USER postgres PASSWORD 'pass1';` (need to find a way to validate this, the issue with the & needing to be an ? from the README may have been the problem the whole time vs the username/password bit, return to this later)

### Accompanying code example

The accompanying Substreams module associated with this documentation is responsible for extracting data from the Ethereum and routing it into a protobuf for persistence in a PostgreSQL database.

The main files you'll need to modify to customize the codebase to your needs include `substreams.yaml`, `lib.rs`, `schema.sql`, `block_meta.proto` and `Cargo.toml`.

## Installation

### Install `substreams-sink-postgres`

Install `substreams-sink-postgres` by using the pre-built binary release [available in the official GitHub repository](https://github.com/streamingfast/substreams-sink-postgres/releases).

Extract `substreams-sink-postgres` into a folder and ensure this folder is referenced globally via your `PATH` environment variable.

### Accompanying code example

The accompanying code example for this tutorial is available in the `substreams-sink-postgres` respository. The Substreams project for the tutorial is located in the `docs/tutorial/` directory.

Run the included `make codegen` command to create the required protobuf files.

```bash
make codegen
```

It's a good idea to run the example code using your installation of the `substreams` CLI to make sure everything is set up and working properly.

Verify the setup for the example project by using the `make build` and `substreams run` commands.

Build the Substreams module by using the included `make` command.

```bash
make
```

Run the project by using the `substreams run` command.

```bash
substreams run -e mainnet.eth.streamingfast.io:443 substreams.yaml db_out --start-block 100 --stop-block +1
```

The `substreams run` command will result in output resembling the following:

```bash
Connected - Progress messages received: 0 (0/sec)
Connected - Progress messages received: 101 (0/sec)
Backprocessing history up to requested target block 100:
(hit 'm' to switch mode)

store_block_meta_start            0  ::  0-99
----------- NEW BLOCK #100 (100) ---------------
all done
```

## Substreams modifications

### Module handler changes for sink

The example code in the [`lib.rs`](#) Rust source code file contains the `db_out` module handler responsible for preparing and returning the module's `DatabaseChanges` output. The data sent out of the Substreams module is captured by the `substreams-sink-postgres` tool. The tool routes the data into the proper columns and tables in the Postgres database.

```rust
#[substreams::handlers::store]
fn store_block_meta_start(blk: ethpb::eth::v2::Block, s: StoreSetIfNotExistsProto<BlockMeta>) {
    let (timestamp, meta) = transform_block_to_block_meta(blk);

    s.set_if_not_exists(meta.number, timestamp.start_of_day_key(), &meta);
    s.set_if_not_exists(meta.number, timestamp.start_of_month_key(), &meta);
}
```

>>>>>>> Stashed changes
---

<b>DEV NOTES</b>

<<<<<<< Updated upstream
TODO: Go through this outine to compare and contrast it to what's currently in the sink files and sink kv documentation. We need to be as consistent as possible.
=======
TODO: Go through this outine to compare and contrast it to what's currently in the sink files and sink kv documentation. We need to be as consistent as possible. It may be best to back-port what we end up with for this page to the KV page. Also, watch out for proper references to the Substreams command-line interface <i>it should always be</i>: [`substreams` CLI](https://substreams.streamingfast.io/reference-and-specs/command-line-interface) to conform to the Google technical writing style guide for documentation, specifically the command-line interface section/rules.
>>>>>>> Stashed changes

Here a first draft outline:

- Overview (discuss about transformation required to fit expected model, that the sink consumes this and populate a database.)
- Prepare your Substreams (how to respect database changes format, examples and explanations, how to check https://github.com/streamingfast/substreams-eth-block-meta for examples of the format).
- Dependencies requires (Docker compose to launch a local Postgres instance, schema population)
- Run and configure substreams-sink-postgres (launching, flags, output, inspect, results)
- Discussion about where Substreams cursor is saved (in a table)
- Discussion about batching of writes (each 1000 blocks when not live, still need to be determined when we will be live)
- Conclusion
