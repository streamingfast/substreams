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

**Important**: Changing the module name does **not** affect the cache key. The cache is based on the module's actual code and data flow, not its name.

### Viewing Module Hash

You can view the module hash for any Substreams package using the `substreams info` command:

```bash
substreams info <spkg> <module_name>
```

The module hash is displayed beside the `Hash:` label in the output. This hash uniquely identifies the module's configuration and is used as the cache key.

## How Caching Works

### First Run

When you run a module for the first time (or after any changes that affect the module hash):

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

## Performance Implications

Module caching has significant effects on performance characteristics:

### Development vs. Production

- **First Run**: Slower, as it requires full WASM execution for all blocks
- **Subsequent Runs**: Much faster, as outputs are simply read from cache
- **Input Dependencies**: If your module depends on other modules as inputs, and those dependencies are cached, your module receives cached inputs without those dependencies being re-executed

### Performance Testing Considerations

{% hint style="warning" %}
**Important:** The first run will always be slower than subsequent runs due to cache population. For accurate performance comparisons, ensure you're comparing runs with the same cache state (either both cached or both uncached).
{% endhint %}

When benchmarking or performance testing Substreams:

- **First run performance** reflects actual WASM execution time and processing logic
- **Cached run performance** reflects I/O throughput and network delivery speed
- For accurate benchmarks, clear caches or use different module versions to measure true execution performance
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
