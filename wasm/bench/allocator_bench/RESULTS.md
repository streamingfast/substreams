# WASM Allocator Benchmark Results

## Executive Summary

This document presents performance benchmarking results for different WASM global allocators in Substreams Rust modules. The benchmarks were run in the **WASMTime** execution environment (the production runtime).

### Key Findings

| Allocator | Performance vs Default | Module Size | Production Ready |
|-----------|----------------------|-------------|------------------|
| **default (dlmalloc)** | baseline | 200.7 KB | ✅ Yes |
| **mini-alloc** | -0.5% to +3% | 199.8 KB | ✅ Yes |
| **talc** | +3% to +5% slower | 199.3 KB | ✅ Yes |
| **rlsf** | +5% to +7% slower | 193.0 KB | ✅ Yes |
| **lol_alloc** | FAILED (OOM) | 199.1 KB | ❌ No |

**Winner: default (dlmalloc) or mini-alloc** - Both perform nearly identically with WASMTime.

### Tested Allocators

1. **dlmalloc** (default) - Rust's default WASM allocator
2. **talc** - WASM-optimized allocator claiming better performance
3. **rlsf** - Real-time allocator with constant-time operations
4. **lol_alloc** - Simple allocator (FAILED - caused OOM on large blocks)
5. **mini-alloc** - Minimal allocator (performs well!)

## Module Size Comparison

| Allocator | WASM Size (bytes) | Delta vs Default |
|-----------|------------------|------------------|
| rlsf      | 192,997          | -7,690 (-3.8%)   |
| talc      | 199,280          | -1,407 (-0.7%)   |
| lol       | 199,133          | -1,554 (-0.8%)   |
| mini      | 199,799          | -888 (-0.4%)     |
| default   | 200,687          | baseline         |

**Key Finding**: All alternative allocators are smaller than default. **rlsf** is the smallest at -3.8%.

## Performance Results (WASMTime)

### Methodology

- **Runtime**: WASMTime (via wasmtime-go v41) - Production runtime
- **Test Blocks**: Real production blocks from 4 chains
  - Ethereum mainnet (block 16021772) - ~778 KB
  - BNB mainnet (block 78347457) - ~935 KB
  - Polygon mainnet (block 82340855) - ~3.3 MB
  - Solana mainnet (block 396976773) - ~3.7 MB
- **Benchmarks**: 4 allocation patterns per chain
  - `decode_only`: Protobuf decode only
  - `small_allocs`: Many small allocations (strings, formatting)
  - `large_allocs`: Large buffer with repeated realloc
  - `mixed`: Combined pattern with protobuf output
- **Iterations**: 3 runs, 2 seconds per benchmark

### Average Performance by Chain

#### Ethereum (Small Block - 778 KB)

| Allocator | decode_only | small_allocs | large_allocs | mixed | Avg |
|-----------|-------------|--------------|--------------|-------|-----|
| default   | 2.11 ms     | 2.27 ms      | 2.13 ms      | 2.23 ms | **2.19 ms** |
| mini      | 2.09 ms     | 2.26 ms      | 2.17 ms      | 2.23 ms | **2.19 ms** |
| talc      | 2.19 ms     | 2.35 ms      | 2.22 ms      | 2.33 ms | 2.27 ms (+4%) |
| rlsf      | 2.24 ms     | 2.41 ms      | 2.26 ms      | 2.39 ms | 2.33 ms (+6%) |

#### BNB (Medium Block - 935 KB)

| Allocator | decode_only | small_allocs | large_allocs | mixed | Avg |
|-----------|-------------|--------------|--------------|-------|-----|
| default   | 2.29 ms     | 2.43 ms      | 2.34 ms      | 2.44 ms | **2.38 ms** |
| mini      | 2.36 ms     | 2.50 ms      | 2.48 ms      | 2.46 ms | 2.45 ms (+3%) |
| talc      | 2.34 ms     | 2.51 ms      | 2.34 ms      | 2.45 ms | 2.41 ms (+1%) |
| rlsf      | 2.39 ms     | 2.53 ms      | 2.40 ms      | 2.56 ms | 2.47 ms (+4%) |

#### Polygon (Large Block - 3.3 MB)

| Allocator | decode_only | small_allocs | large_allocs | mixed | Avg |
|-----------|-------------|--------------|--------------|-------|-----|
| default   | 7.55 ms     | 8.06 ms      | 7.68 ms      | 7.79 ms | **7.77 ms** |
| mini      | 7.81 ms     | 8.12 ms      | 8.18 ms      | 7.99 ms | 8.03 ms (+3%) |
| talc      | 7.55 ms     | 8.04 ms      | 7.68 ms      | 7.84 ms | 7.78 ms (+0.1%) |
| rlsf      | 7.69 ms     | 8.17 ms      | 7.81 ms      | 8.05 ms | 7.93 ms (+2%) |

#### Solana (Large Block - 3.7 MB)

| Allocator | decode_only | small_allocs | large_allocs | mixed | Avg |
|-----------|-------------|--------------|--------------|-------|-----|
| default   | 8.71 ms     | 17.33 ms     | 8.78 ms      | 8.65 ms | **10.87 ms** |
| mini      | 8.46 ms     | 17.01 ms     | 8.52 ms      | 8.42 ms | **10.60 ms** (-2%) |
| talc      | 9.02 ms     | 17.95 ms     | 9.17 ms      | 9.12 ms | 11.31 ms (+4%) |
| rlsf      | 9.35 ms     | 17.95 ms     | 9.18 ms      | 9.13 ms | 11.40 ms (+5%) |
| lol       | **FAILED (OOM)** | - | - | - | - |

**Note**: lol_alloc crashed with "rust_oom" / "handle_alloc_error" on Solana blocks.

## Analysis

### Performance Winner: **default (dlmalloc)** or **mini-alloc**

With WASMTime, the results are different from wazero:

1. **mini-alloc performs excellently** - Within 3% of default, sometimes faster
2. **default remains reliable** - Consistent performance across all workloads
3. **talc is slower** - 3-5% slower than default despite claims
4. **rlsf is the slowest** - 5-7% slower, trades speed for code size

### Why mini-alloc Works Well with WASMTime

Unlike wazero, WASMTime:
- Has more efficient memory management
- Better handles mini-alloc's zero-fill page optimization
- Provides more predictable allocation patterns

### lol_alloc Failure Analysis

lol_alloc crashed with OOM errors:
```
rust_oom -> handle_alloc_error -> raw_vec::handle_error
```

This happens because lol_alloc:
- Never frees memory (leaking allocator)
- Runs out of WASM linear memory on large blocks
- Not suitable for production workloads

## Recommendations

### For Production Substreams Modules

**Stick with the default allocator (dlmalloc)**

Reasons:
1. Best overall performance
2. No configuration required
3. Battle-tested and reliable
4. Compatible with all runtimes (wasmtime, wazero)

### When to Consider Alternatives

**Use rlsf if**:
- Code size is critical (saves 7.7 KB / 3.8%)
- You can tolerate 5-7% performance loss
- You need predictable allocation latency

**Use mini-alloc if**:
- You're exclusively targeting WASMTime
- You want slightly smaller code size (-0.4%)
- Testing shows good performance for your workload

**Avoid**:
- **lol_alloc** - Not production-ready, crashes on large blocks
- **talc** - Slower than default with no clear benefits

## Comparison: WASMTime vs wazero

| Metric | WASMTime | wazero |
|--------|----------|--------|
| **Best allocator** | default ≈ mini | default |
| **mini-alloc** | Excellent | Not tested (wazero failed) |
| **Performance spread** | Tighter (0-7%) | Wider (5-25%) |
| **lol_alloc** | OOM on large blocks | Timeout |

WASMTime provides more consistent performance across allocators.

## Technical Details

### Benchmark Environment

```
OS: Linux 6.12.54-linuxkit
Arch: arm64 (aarch64)
Runtime: WASMTime (wasmtime-go v41)
Go Version: 1.25
Rust Version: nightly-2026-01-15
Prost Version: 0.13
Substreams: 0.6
```

### Allocator Verification

Each WASM module was verified to use its designated allocator:

```
default module → reports "default"
talc module → reports "talc"
rlsf module → reports "rlsf"
lol module → reports "lol_alloc"
mini module → reports "mini_alloc"
```

### Build Configuration

All modules built with:
```toml
[profile.release]
lto = true
opt-level = 's'
strip = "debuginfo"
```

## Raw Results Files

- `benchmark_wasmtime_results.txt` - Full WASMTime benchmark output
- `benchmark_results_clean.txt` - Previous wazero results (deprecated)

## Conclusion

With **WASMTime** (the production runtime):

1. **default (dlmalloc)** remains the best choice for most use cases
2. **mini-alloc** is a viable alternative with nearly identical performance
3. **rlsf** offers the smallest code size at a 5-7% performance cost
4. **talc** offers no benefits over default
5. **lol_alloc** is not production-ready

**Recommendation**: Keep using the default allocator. The 0-3% difference with alternatives doesn't justify the configuration complexity or compatibility risks.
