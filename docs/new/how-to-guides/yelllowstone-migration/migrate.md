# Migrate From Yellowstone to Substreams

## Introduction

Both Substreams and Yellowstone allow you consume Solana data in a fast and reliable way through gRPC connection. However, there are some unique capabilities of Substreams that make it shine:

### Improvements over Yellowstone

- Substreams is a programmable stack, while Yellowstone is only a gRPC interface with the Geyser plugin.
- Substreams gives you access to the full Solana `Block`, and you can use Rust to filter and output the schema that you need.
- Substreams runs on top of a parallelization engine, which speeds up the indexing times.
- Substreams allows you to filter data and create your own output schema (you choose what data model gets ouputted from the Substreams).
- Substreams is a composable stack, which means that you can reuse other Substreams modules built by other people (take a look at the [Substreams Registry](https://substreams.dev)).
- Substreams has native integrations with many _sinks_ (people where you want to consume the data), such as Postgres or Subgraphs. You can also use libraries like Go, Rust or JavaScript.

### Pricing

Yellowstone is usually charged based on credit units. A single response from Yellowstone will cost X credit units.

In Substreams, you are charged depending on the amount of data (TBs) that you consume from the endpoint. Therefore, you will be charge exactly for what the Solana blockchain is producing.

To reduce the cost even more, we have caches of data that will help you consume less data (blocks without voting transactions cache or transactions filtered by program ID cache, for example).

## Example: Migrating

Substreams offers two different endpoints to track Solana data:
- Solana Block data (transactions, instructions, logs...): ``
- Solana Account Changes (historical account data): ``

There are three main parts in the Substreams development:
1. Initialize your Substreams project (using the Substreams CLI).
2. Use Rust to filter the data that you need (or choose an existing Substreams that outputs the data that you need).
3. Consume the data in a variety of _sinks_ (SQL, Subgraphs, streaming...).

Developing Substreams involves three main dependencies: the Rust programming language, the Buf CLI (Protobuf), and the Substreams CLI (a command-line utility tool to manage your Substreams). Follow [this guide](../../references/cli/installing-the-cli.md) to install all the necessary dependencies in your local computer OR use the [DevContainer](https://github.com/streamingfast/substreams-starter), which contains all the necessary dependencies ready to use.

### 1. Initialize your Substreams project.

The `substreams init` command allows you to boostrap your Substreams project for a variety of chains, including Solana.

### 2. Filter the Data

Once you have your project created, you can modify the `src/lib.rs` file to narrow the data that you want to 

### 3. Consume the Data

