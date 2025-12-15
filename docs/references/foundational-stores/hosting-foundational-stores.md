# Hosting a Foundational Store

This guide explains how to host a Foundational Store for Substreams data processing pipelines. Foundational Stores provide persistent, fork-aware key-value storage that can be populated by Substreams modules and served to other modules.

## What is a Foundational Store?

A Foundational Store is a high-performance, multi-backend key-value storage system designed for Substreams ingestion and serving. It provides:

- **Fork-aware storage**: Handles blockchain reorganizations automatically
- **Multiple backends**: Supports Badger (embedded) and PostgreSQL
- **Block-level versioning**: Every entry tagged with block number for historical queries
- **High-performance serving**: gRPC API for data retrieval
- **Streaming ingestion**: Continuous processing via Substreams sink

Foundational Stores are typically populated by Substreams modules that extract and transform blockchain data, then serve that data to other Substreams modules for efficient lookups.

## Hosting a Foundational Store

To host a Foundational Store, you need:

1. A Substreams module that produces the data to store
2. The `foundational-store` binary
3. A storage backend (Badger or PostgreSQL)

### Step 1: Prepare Your Data Source

Your Substreams module should output data in the format expected by the Foundational Store. The data is stored as key-value pairs with block-level versioning.

Example: The [SPL Initialized Account module](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/solana/spl-initialized-account) tracks Solana SPL token account initializations and stores account-to-owner mappings.

### Step 2: Build the Foundational Store Binary

```bash
git clone https://github.com/streamingfast/substreams-foundational-store
cd substreams-foundational-store
go build -o foundational-store ./cmd/foundational-store
```

### Step 3: Run the Server

Start the server with your Substreams module as the data source:

```bash
./foundational-store server \
  --dsn "badger:///path/to/data" \
  --type-url "your.module.Type" \
  --manifest-path "../your-substreams-module/substreams.yaml" \
  --output-module-name "your_output_module" \
  --endpoint "your-blockchain.streamingfast.io:443" \
  --start-block 1000000
```

**Configuration Options:**

- `--dsn`: Storage backend (Badger: `badger:///path`, PostgreSQL: `postgres://user:pass@host/db`)
- `--type-url`: Protobuf type URL for stored values
- `--manifest-path`: Path to your Substreams manifest
- `--output-module-name`: Name of the output module in your Substreams
- `--endpoint`: StreamingFast endpoint for blockchain data
- `--start-block`: Block to start ingestion from

### Step 4: Monitor and Maintain

The server exposes:
- gRPC API on port 50051 (default)
- Prometheus metrics on port 9102 (default)
- Health checks via gRPC reflection

Monitor ingestion progress through cursor files and Prometheus metrics.

## Using SinkEntries for Data Ingestion

Foundational Stores ingest data through `SinkEntries` messages, which allow batch operations with conditional insertion.

### SinkEntries Structure

```protobuf
message SinkEntries {
  repeated Entry entries = 1;
  bool if_not_exist = 2;
}
```

- `entries`: Array of key-value pairs to store
- `if_not_exist`: Controls insertion behavior

### Conditional Insertion with if_not_exist

The `if_not_exist` flag determines how entries are inserted:

- **When `false` (default)**: Entries are inserted regardless of existing keys, potentially overwriting previous values
- **When `true`**: Entries are only inserted if the key doesn't already exist in the store

The check is performed against both the in-memory cache and persistent storage to ensure consistency.

### Example: Producing Data for a Foundational Store

The [spl-initialized-account](https://github.com/streamingfast/substreams-foundational-modules/tree/develop/solana/spl-initialized-account) module shows how to produce data that populates a Foundational Store.

It processes Solana transactions and outputs account-owner mappings as `SinkEntries`:

```rust
#[substreams::handlers::map]
fn map_initialize_account(txn: Transaction) -> Result<SinkEntries, Error> {
    let mut entries = vec![];

    // Extract account initialization data
    let account = txn.accounts[0];
    let owner = extract_owner_from_instruction(&txn)?;

    // Create entry with conditional insertion to prevent duplicates
    entries.push(Entry {
        key: Key { bytes: account.to_bytes().to_vec() },
        value: Any::pack(&AccountOwner {
            account: account.clone(),
            owner,
        })?,
    });

    Ok(SinkEntries {
        entries,
        if_not_exist: true,  // Only store if account not already initialized
    })
}
```

This data is then ingested by the Foundational Store sink for later querying.

## Best Practices

### Hosting
- Configure appropriate batch sizes and flush intervals for your throughput needs
- Implement proper backup strategies for your storage backend

### Performance Tuning
- Adjust `--batch-size` and `--max-batch-time` based on your data volume
- Use SSD storage for Badger backend
