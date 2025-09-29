---
description: Get started with foundational stores
---

# Using your first Foundational Store

This tutorial walks you through the essential steps to consume foundational stores in your Substreams modules, from adding the input to handling responses.

## Step 1: Add foundational store input

First, add the foundational store as an input to your module in the manifest `substreams.yaml`:

```yaml
modules:
  - name: map_my_data
    kind: map
    ...
    inputs:
      ...
       - foundational-store: my-store@v0.1.0
```

## Step 2: Use FoundationalStore in your handler

Add the foundational store parameter to your Rust function:

```rust
#[substreams::handlers::map]
fn map_my_data(
    ...,
    my_store: FoundationalStore,
) -> Result<Output, Error> {
    // Your logic here
}
```

## Step 3.1: Single key queries with Get

The [`get` function](https://github.com/streamingfast/substreams-rs/blob/develop/substreams/src/store.rs#L1651) retrieves a single value by its key. When you call get, you provide a key and the foundational store returns:

- **Response code**: Whether the key was found or not
- **Value**: The actual data stored (if found)
- **Block context**: The store automatically uses the current block being processed

## Step 3.2: Multiple key queries with GetAll

The [`get_all` function](https://github.com/streamingfast/substreams-rs/blob/develop/substreams/src/store.rs#L1674) retrieves multiple values in a single operation. This is much more efficient than making multiple get calls because:

- **Single operation**: All keys processed together
- **Batch processing**: Reduces overhead significantly
- **Consistent view**: All keys queried at the same block height

## Step 4: Understanding response codes

Foundational stores return detailed status information for each query:

- **FOUND**: Key exists and value retrieved successfully
- **NOT_FOUND**: Key doesn't exist at the current block
- **NOT_FOUND_FINALIZE**: Key was deleted after finality
- **NOT_FOUND_BLOCK_NOT_REACHED**: Block hasn't been processed yet

## When to use Get vs GetAll

**Use Get when:**
- You need a single piece of data
- The query depends on previous results
- You're doing conditional lookups

**Use GetAll when:**
- You need multiple related pieces of data
- You know all keys upfront
- Performance is critical

## Next steps

- [Chain-specific foundational stores](../how-to-guides/foundational-stores/) for your blockchain
- [Foundational store architecture](../references/foundational-stores.md) for technical details
