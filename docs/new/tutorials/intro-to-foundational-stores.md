---
description: Get started with foundational stores
---

# Using your first Foundational Store

This tutorial gives you an overview of what foundational stores can do and how to interact with them within your Substreams modules.

## Single key queries with Get

The `get` function retrieves a single value by its key. When you call get, you provide a key and the foundational store returns:

- **Response code**: Whether the key was found or not
- **Value**: The actual data stored (if found)
- **Block context**: The store automatically uses the current block being processed

## Multiple key queries with GetAll

The `getall` function retrieves multiple values in a single operation. This is much more efficient than making multiple get calls because:

- **Single operation**: All keys processed together
- **Batch processing**: Reduces overhead significantly
- **Consistent view**: All keys queried at the same block height

## Understanding response codes

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
