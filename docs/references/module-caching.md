---
description: Learn how Substreams modules are cached for efficient execution
---

# Module Caching

Module caching is a fundamental feature of Substreams that significantly improves performance by storing the output of module executions. Once a module has been executed for a specific block, its output is cached, and subsequent requests for the same block will read from the cache instead of re-executing the WASM code.

## Overview

Every Substreams module is cached based on a unique identifier called a **module hash**. This hash is computed from the module's WASM code, inputs, outputs, and additional configuration parameters. When you run a module, Substreams uses this hash to determine whether to execute the WASM code or retrieve cached results.

## Cache Key Computation

The cache key (module hash) is computed from:
- Module's WASM bytecode
- Module inputs
- Module outputs  
- Additional module metadata

**Important**: Changing the module name or the `.spkg` package name does **not** affect the cache key. The cache is based on the module's actual code and data flow, not its name or the package name.

### Viewing Module Hash

You can view the module hash for any Substreams package using the `substreams info` command:

```bash
substreams info <spkg> <module_name>
```

For example:

```bash
substreams info common@latest map_clocks
```

Output:
```
...
Modules:
----
Name: map_clocks
Hash: 7685a04836b6bac7ec654589bf1fe79ec3decbe1
...
```

The module hash is displayed beside the `Hash:` label. This hash uniquely identifies the module's configuration and is used as the cache key.

## How Caching Works

{% hint style="info" %}
**Note:** Module caching occurs only in **production mode**. In **development mode**, Substreams always re-executes the code and does not use or populate the cache. This ensures you're always testing the latest version of your code during development.
{% endhint %}

### First Run

When you run a module for the first time in production mode (or after any changes that affect the module hash):

1. The Substreams engine executes the Rust (WASM) code of your module
2. The module processes each block with its inputs
3. The module emits outputs for each block
4. The output for each block is **written to disk** (cached) based on the module hash
5. The output is also **streamed to you** in real-time

### Subsequent Runs

When you run the same module again (with the same module hash):

1. For each block, Substreams checks if cached data exists for that module hash
2. If cached data is found:
   - The Rust (WASM) code is **skipped entirely**
   - The cached output is read from disk
   - The cached output is streamed to you
3. If no cached data exists for a specific block, the module executes normally for that block

This caching mechanism applies to **all module types**: maps, stores, and indexes.

## Module Types and Caching

Caching applies uniformly across all module types:

- **Maps**: Cached outputs are read instead of executing WASM code
- **Stores**: Cached state is loaded instead of recomputing aggregations
- **Indexes**: Cached index data is reused for filtering

Once a module has been executed for a block range, subsequent requests for the same module (identified by its hash) will retrieve pre-computed results instead of re-executing.

## Stores and Cache Dependencies

Store modules require special consideration when it comes to caching:

### Store Backfilling

**Stores always need to be backfilled** from their initial block to be usable. This makes caching for stores particularly important compared to maps and indexes, as stores accumulate state over time and rebuilding them from scratch can be time-consuming.

### WASM Binary Hash Impact

The module hash is computed from the **WASM binary code**. This has an important implication: **changing a single line of Rust code invalidates the hash of all modules that depend on that code**, since the WASM binary will be different.

This affects store caching significantly. If you have a store module and change any shared Rust code it uses, the store's hash changes, and all cached data becomes invalid. The store will need to be completely re-backfilled.

### Designing for Efficient Store Caching

When designing Substreams with stores that need to be cached efficiently:

**Option 1: Split into Multiple Substreams Packages**
- Create separate `.spkg` files for stable store modules
- Use these packages as inputs to other modules
- Changes to consuming modules won't affect the store's hash
- The store remains cached even when you modify other parts of your system

**Option 2: Split into Different WASM Files**
- Separate frequently-changing code from stable store logic
- Note: This is less reliable if you have shared "common" code, as changes to common code still affect all modules using it

This architectural decision is crucial for projects where store re-computation is expensive and you need to iterate quickly on dependent modules.

## Performance Implications

Module caching has significant effects on performance characteristics:

- **First Run (Production Mode)**: Slower, as it requires full WASM execution for all blocks
- **Subsequent Runs (Production Mode)**: Much faster, as outputs are simply read from cache
- **Development Mode**: Always executes WASM code, never uses cache
- **Input Dependencies**: If your module depends on other modules as inputs, and those dependencies are cached, your module receives cached inputs without those dependencies being re-executed

### Performance Testing Considerations

{% hint style="warning" %}
**Important:** The first run will always be slower than subsequent runs due to cache population. For accurate performance comparisons, ensure you're comparing runs with the same cache state (either both cached or both uncached).
{% endhint %}

When benchmarking or performance testing Substreams:

- **First run performance** reflects actual WASM execution time and processing logic
- **Cached run performance** reflects I/O throughput and network delivery speed
- To measure true execution performance, you need to invalidate the cache by changing the module hash (e.g., making a small change to the Rust code), as there is currently no direct cache-clearing mechanism
- Production deployments benefit from pre-cached data, making the first runs important for cache warming

### Cache Behavior with Module Changes

Any change that affects the module hash will invalidate the cache:

- Modifying the WASM code (Rust implementation)
- Changing module inputs
- Changing module outputs
- Modifying module configuration parameters

When the module hash changes, Substreams treats it as a completely new module and builds a fresh cache.

## Best Practices

### Module Naming
Changing a module's name does not affect its cache. The cache key is based on the module hash (computed from code, inputs, and outputs), not the name. You can safely rename modules without invalidating cached data.

### Composability
Leverage caching by building on existing modules. If you import and use a module that's already cached on the server, your new module can benefit from those cached inputs, significantly reducing processing time.

### Testing
When testing module changes, be aware that cached data from previous versions won't be used for the new version. Each unique module hash has its own cache.

### Cache Warming
For production deployments, consider running modules in advance to populate caches and reduce latency for end users. The first execution of a module (or module chain) will always be slower as it builds the cache.

## Related Concepts

- [Architecture & Parallel Execution](architecture.md) - Learn how caching interacts with parallel execution
- [Module Concepts](substreams-components/modules/modules.md) - Understand the different types of modules and how they work
- [Reliability Guarantees](reliability-guarantees.md) - Learn about determinism and consistency in Substreams
