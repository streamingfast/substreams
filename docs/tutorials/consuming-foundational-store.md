# Consuming a Foundational Store

This guide explains how to consume data from a Foundational Store in your Substreams modules. Foundational Stores provide efficient access to pre-processed blockchain data for building complex data processing pipelines.

## What is a Foundational Store?

A Foundational Store is a high-performance, multi-backend key-value storage system designed for Substreams ingestion and serving. It provides:

- **Fork-aware storage**: Handles blockchain reorganizations automatically
- **Multiple backends**: Supports Badger (embedded) and PostgreSQL
- **Block-level versioning**: Every entry tagged with block number for historical queries
- **High-performance serving**: gRPC API for data retrieval
- **Streaming ingestion**: Continuous processing via Substreams sink

Foundational Stores are typically populated by Substreams modules that extract and transform blockchain data, then serve that data to other Substreams modules for efficient lookups.

## Consuming a Foundational Store

To consume data from a Foundational Store in your Substreams module:

### Step 1: Import the Foundational Store

Add the import to your `substreams.yaml`:

```yaml
imports:
  your_store: your-foundational-store@v1.0.0

modules:
  - name: your_module
    kind: map
    inputs:
      - foundational-store: your_store
    output:
      type: proto:your.OutputType
```

### Step 2: Query the Store in Code

Use the `FoundationalStore` input in your Rust handler:

```rust
use substreams::store::FoundationalStore;

#[substreams::handlers::map]
fn process_data(foundational_store: FoundationalStore) -> Result<YourOutput, Error> {
    // Single key lookup
    let response = foundational_store.get(key_bytes);
    if response.response == ResponseCode::Found as i32 {
        // Process found data
    }

    // Batch lookup (recommended for performance)
    let keys = vec![key1, key2, key3];
    let queriedEntries = foundational_store.get(keys);
    for entry in queriedEntries.entries {
        // Process each entry
    }

    Ok(your_output)
}
```

## Understanding QueriedEntries Responses

When querying a Foundational Store, responses are returned as `QueriedEntries` containing multiple `QueriedEntry` results.

### QueriedEntries Structure

```protobuf
message QueriedEntries {
  repeated QueriedEntry entries = 2;
}

message QueriedEntry {
  ResponseCode code = 1;
  Entry entry = 2;
}
```

Each `QueriedEntry` corresponds to one requested key, in the same order as the request.

### Response Codes

- `RESPONSE_CODE_FOUND`: Key exists and value was retrieved successfully
- `RESPONSE_CODE_NOT_FOUND`: Key does not exist at the requested block
- `RESPONSE_CODE_NOT_FOUND_FINALIZE`: Key was deleted after finality (LIB) - historical reference only
- `RESPONSE_CODE_UNSPECIFIED`: Default value, should not occur in normal operation

### Handling Responses in Code

```rust
use substreams::store::FoundationalStore;
use sf::substreams::foundational_store::model::v2::{QueriedEntry, ResponseCode};

#[substreams::handlers::map]
fn process_data(foundational_store: FoundationalStore) -> Result<YourOutput, Error> {
    let keys = vec![key1, key2, key3];
    let response = foundational_store.get_all(keys);

    for queried_entry in response.entries {
        match queried_entry.code {
            x if x == ResponseCode::Found as i32 => {
                // Successfully found - unpack the value
                let account_owner: AccountOwner = queried_entry.entry
                    .value
                    .unpack()?;
                // Process the data
            }
            x if x == ResponseCode::NotFound as i32 => {
                // Key doesn't exist - handle missing data
                substreams::log::debug!("Key not found");
            }
            x if x == ResponseCode::NotFoundFinalize as i32 => {
                // Key was deleted after finality
                substreams::log::info!("Key deleted after finality");
            }
            _ => {
                // Handle unexpected response codes
                substreams::log::error!("Unexpected response code: {}", queried_entry.code);
            }
        }
    }

    Ok(your_output)
}
```

## Example: SPL Token Transfers with Ownership Resolution

The [substreams-spl-token](https://github.com/streamingfast/substreams-spl-token) module demonstrates consuming a Foundational Store.

It imports the SPL Initialized Account store to resolve token account ownership:

```yaml
imports:
  spl_initialized_account: spl-initialized-account@v0.1.2

modules:
  - name: map_spl_instructions
    inputs:
      - foundational-store: spl_initialized_account
```

Then queries the store to get owner addresses for transfer resolution:

```rust
#[substreams::handlers::map]
fn map_spl_instructions(spl_initialized_account_store: FoundationalStore) -> Result<SplInstructions, Error> {
    // Resolve owners for transfer accounts
    let from_owner = get_owner(&spl_initialized_account_store, &transfer.from)?;
    let to_owner = get_owner(&spl_initialized_account_store, &transfer.to)?;

    // Create transfer with resolved ownership
    transfers.push(Transfer {
        from_owner,
        to_owner,
        amount: transfer.amount,
        // ...
    });
}
```
