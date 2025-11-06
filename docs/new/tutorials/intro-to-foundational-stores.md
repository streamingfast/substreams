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
## Step 3: Multiple key queries with GetAll

The [`get_all` function](https://github.com/streamingfast/substreams-rs/blob/develop/substreams/src/store.rs#L1674) retrieves multiple values in a single operation. This is much more efficient than making multiple get calls because:

- **Single operation**: All keys processed together
- **Batch processing**: Reduces overhead significantly
- **Consistent view**: All keys queried at the same block height

A variant, [`get`](https://github.com/streamingfast/substreams-rs/blob/develop/substreams/src/store.rs#L1651) is available if you need to query a single key per block.

## Step 4: Understanding response codes

Foundational stores return detailed status information for each query:

- **FOUND**: Key exists and value retrieved successfully
- **NOT_FOUND**: Key doesn't exist at the current block
- **NOT_FOUND_FINALIZE**: Key was deleted after finality
- **NOT_FOUND_BLOCK_NOT_REACHED**: Block hasn't been processed yet

## Next steps

- [Foundational store ](https://github.com/streamingfast/substreams-foundational-store/blob/develop/README.md) for technical details
