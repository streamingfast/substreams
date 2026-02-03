# WASM32 Allocator Benchmarking Plan

## ULTIMATE GOAL

Evaluate and compare different WASM32 global allocators for Rust Substreams modules to optimize performance. The benchmarks must run within the actual WASMTime execution environment (not native Rust) to get accurate measurements for the real Substreams workload characteristics.

## Status: COMPLETED

## Results Summary

**Winner: dlmalloc (default allocator)**

After comprehensive benchmarking in the actual WASM runtime environment (wazero), the default allocator outperforms all alternatives:
- **5-25% faster** execution time across all workloads
- **Lowest memory usage**
- **Battle-tested reliability**

Alternative allocators showed NO performance benefit:
- **talc**: 5-12% slower, larger code size (+2.5%)
- **rlsf**: 12-24% slower, only advantage is smallest code (-0.4%)
- **lol_alloc**: FAILED (OOM/timeout), not production-ready
- **mini-alloc**: Not tested due to lol_alloc failure

**Recommendation**: Keep using the default allocator. See `wasm/bench/allocator_bench/RESULTS.md` for full analysis.

---

## Context

We want to evaluate different global allocators for Rust WASM32 targets to optimize Substreams module performance. The key challenge is that allocator performance on native platforms (x86_64, aarch64) does **not** reflect WASM32 performance due to fundamental differences in memory models.

### Why WASM32 is Different

- **Linear memory model**: WASM memory grows in 64KB pages, no virtual memory
- **No threading** (in most cases): Many native allocator optimizations don't apply
- **Different syscall overhead**: Memory growth via `memory.grow` instruction
- **Code size matters**: Larger allocators increase WASM module size and load time

### Our Workload

Substreams modules heavily use:
- **Protobuf encoding/decoding**: Many small-to-medium allocations for fields
- **Large protobuf messages**: Some messages can be KB to MB in size
- **Repeated processing**: Same patterns executed millions of times per block range

---

## WASM32 Allocator Candidates

### 1. **dlmalloc** (Default)
- **Status**: Current default for `wasm32-unknown-unknown`
- **Pros**: Battle-tested, good general performance
- **Cons**: Larger code size (~50% of small WASM modules)
- **Source**: Built into Rust std for WASM

### 2. **Talc**
- **Crate**: [`talc`](https://crates.io/crates/talc)
- **GitHub**: https://github.com/SFBdragon/talc
- **Pros**:
  - Claims to be both smaller AND faster than dlmalloc for WASM
  - O(1) deallocation, O(1) in-place reallocation
  - Actively maintained, WASM-focused benchmarks included
  - Stable Rust support (MSRV 1.67.1)
- **Cons**: Newer, less battle-tested
- **Notes**: Has built-in WASM benchmarks (`wasm-bench.sh`)

### 3. **rlsf** (TLSF algorithm)
- **Crate**: [`rlsf`](https://crates.io/crates/rlsf)
- **GitHub**: https://github.com/yvt/rlsf
- **Pros**:
  - Constant-time allocation AND deallocation (real-time suitable)
  - Small code size
  - Well-documented algorithm (academic paper backed)
- **Cons**:
  - No concurrent access support (must lock entire pool)
  - Does not return memory pages to system
- **Notes**: Use `SmallGlobalTlsf` for WASM32

### 4. **lol_alloc**
- **Crate**: [`lol_alloc`](https://crates.io/crates/lol_alloc)
- **Pros**: Extremely simple, tiny code size
- **Cons**:
  - Optimized for simplicity, NOT runtime performance
  - WASM32-only
  - Not production ready (per maintainer)
- **Notes**: Good baseline for "minimal allocator" comparison

### 5. **mini-alloc**
- **Crate**: [`mini-alloc`](https://crates.io/crates/mini-alloc)
- **Pros**:
  - Claims 2x cheaper than wee_alloc
  - Exploits WASM zero-filled pages
- **Cons**: NOT safe for WASM multithreading
- **Notes**: Newer option, worth testing

### 6. **wee_alloc** (NOT RECOMMENDED)
- **Crate**: [`wee_alloc`](https://github.com/rustwasm/wee_alloc)
- **Status**: **UNMAINTAINED** - has known memory leak issues
- **Notes**: Skip this one

---

## Implementation Tasks

### Priority 1: Create Benchmark Module (Rust Side)

#### Task 1.1: Create dedicated allocator benchmark module
- [ ] Create new directory: `wasm/bench/allocator_bench/`
- [ ] Single module with both Ethereum and Solana protobuf types
- [ ] Different entrypoints per chain (e.g., `bench_decode_only_ethereum`, `bench_decode_only_solana`)
- [ ] Structure:
  ```
  wasm/bench/allocator_bench/
  ├── Cargo.toml
  ├── Makefile
  ├── rust-toolchain.toml
  ├── substreams.yaml
  ├── buf.gen.yaml
  └── src/
      ├── lib.rs
      ├── allocators.rs       # Allocator selection via features
      ├── bench_ethereum.rs   # Ethereum/BNB/Polygon handlers
      ├── bench_solana.rs     # Solana handlers
      └── pb/
          ├── mod.rs
          ├── ethereum.rs     # sf.ethereum.type.v2
          └── solana.rs       # sf.solana.type.v1
  ```

#### Task 1.2: Create Cargo.toml with allocator feature flags
- [ ] File: `wasm/bench/allocator_bench/Cargo.toml`
- [ ] Content:
  ```toml
  [package]
  name = "allocator-bench"
  version = "1.0.0"
  edition = "2021"

  [lib]
  name = "substreams"
  crate-type = ["cdylib"]

  [features]
  default = []
  alloc-talc = ["talc"]
  alloc-rlsf = ["rlsf"]
  alloc-lol = ["lol_alloc"]
  alloc-mini = ["mini-alloc"]

  [dependencies]
  substreams = "0.6"
  prost = "0.11"
  prost-types = "0.11"
  hex = "0.4"
  bs58 = "0.5"  # For Solana base58 encoding

  # Allocator dependencies (optional)
  talc = { version = "4", optional = true }
  rlsf = { version = "0.2", optional = true }
  lol_alloc = { version = "0.4", optional = true }
  mini-alloc = { version = "0.5", optional = true }

  [profile.release]
  lto = true
  opt-level = 's'
  strip = "debuginfo"
  ```

#### Task 1.3: Create allocator selection module
- [ ] File: `wasm/bench/allocator_bench/src/allocators.rs`
- [ ] Content:
  ```rust
  //! Global allocator selection based on Cargo features.
  //!
  //! Only one allocator feature should be enabled at a time.
  //! If no feature is enabled, the default dlmalloc is used.

  #[cfg(all(target_arch = "wasm32", feature = "alloc-talc"))]
  mod talc_allocator {
      use talc::{Talc, TalcWasm, Talck};

      static mut ARENA: [u8; 0] = [];

      #[global_allocator]
      static ALLOCATOR: Talck<spin::Mutex<()>, TalcWasm> =
          Talc::new(unsafe { talc::ClaimOnOom::new(TalcWasm::new()) }).lock();
  }

  #[cfg(all(target_arch = "wasm32", feature = "alloc-rlsf"))]
  mod rlsf_allocator {
      use rlsf::SmallGlobalTlsf;

      #[global_allocator]
      static ALLOCATOR: SmallGlobalTlsf = SmallGlobalTlsf::new();
  }

  #[cfg(all(target_arch = "wasm32", feature = "alloc-lol"))]
  mod lol_allocator {
      use lol_alloc::LeakingPageAllocator;

      #[global_allocator]
      static ALLOCATOR: LeakingPageAllocator = LeakingPageAllocator;
  }

  #[cfg(all(target_arch = "wasm32", feature = "alloc-mini"))]
  mod mini_allocator {
      #[global_allocator]
      static ALLOCATOR: mini_alloc::MiniAlloc = mini_alloc::MiniAlloc::INIT;
  }
  ```

#### Task 1.4: Create benchmark handlers
- [ ] File: `wasm/bench/allocator_bench/src/lib.rs` - Main entry point
- [ ] File: `wasm/bench/allocator_bench/src/bench_ethereum.rs` - Ethereum/BNB/Polygon handlers
- [ ] File: `wasm/bench/allocator_bench/src/bench_solana.rs` - Solana handlers
- [ ] **IMPORTANT**: Use full `substreams::output` pattern (not macros) for realistic benchmarks
- [ ] Entrypoints follow pattern: `bench_{test}_{chain}` (e.g., `bench_decode_only_ethereum`)

**lib.rs:**
```rust
mod allocators;
mod pb;
mod bench_ethereum;
mod bench_solana;

// Re-export all benchmark entrypoints
pub use bench_ethereum::*;
pub use bench_solana::*;
```

**bench_ethereum.rs** (for Ethereum, BNB, Polygon - all use `sf.ethereum.type.v2.Block`):
```rust
use crate::pb::sf::ethereum::r#type::v2::Block;
use crate::pb::{BenchOutput, LargeOutput, TxData, CallData};
use substreams::errors::Error;

/// Benchmark 1: Protobuf decode only
#[no_mangle]
pub extern "C" fn bench_decode_only_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        Ok(BenchOutput {
            count: blk.transaction_traces.len() as u64,
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 2: Many small allocations
#[no_mangle]
pub extern "C" fn bench_small_allocs_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut strings = Vec::with_capacity(1000);

        for trx in &blk.transaction_traces {
            strings.push(format!("tx:{}", hex::encode(&trx.hash)));
            strings.push(format!("from:{}", hex::encode(&trx.from)));
            strings.push(format!("to:{}", hex::encode(&trx.to)));

            for call in &trx.calls {
                strings.push(format!("call:{}", call.index));
                for log in &call.logs {
                    strings.push(format!("log:{}", log.index));
                }
            }
        }

        Ok(BenchOutput {
            count: strings.len() as u64,
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 3: Large allocation with realloc
#[no_mangle]
pub extern "C" fn bench_large_allocs_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut data = Vec::new();

        for trx in &blk.transaction_traces {
            data.extend_from_slice(&trx.hash);
            data.extend_from_slice(&trx.input);
            for call in &trx.calls {
                data.extend_from_slice(&call.input);
                data.extend_from_slice(&call.return_data);
            }
        }

        Ok(BenchOutput {
            count: data.len() as u64,
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 4: Mixed allocation pattern
#[no_mangle]
pub extern "C" fn bench_mixed_ethereum(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<LargeOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut output = LargeOutput::default();

        for trx in &blk.transaction_traces {
            let mut tx_data = TxData {
                hash: hex::encode(&trx.hash),
                from: hex::encode(&trx.from),
                to: hex::encode(&trx.to),
                ..Default::default()
            };

            for call in &trx.calls {
                tx_data.calls.push(CallData {
                    index: call.index,
                    input_size: call.input.len() as u64,
                    return_size: call.return_data.len() as u64,
                });
            }

            output.transactions.push(tx_data);
        }

        Ok(output)
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}
```

**bench_solana.rs** (similar structure for Solana):
```rust
use crate::pb::sf::solana::r#type::v1::Block;
use crate::pb::{BenchOutput, LargeOutput};
use substreams::errors::Error;

/// Benchmark 1: Protobuf decode only
#[no_mangle]
pub extern "C" fn bench_decode_only_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        Ok(BenchOutput {
            count: blk.transactions.len() as u64,
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 2: Many small allocations
#[no_mangle]
pub extern "C" fn bench_small_allocs_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut strings = Vec::with_capacity(1000);

        for trx in &blk.transactions {
            if let Some(transaction) = &trx.transaction {
                strings.push(format!("sig:{}", bs58::encode(&transaction.signatures.first().unwrap_or(&vec![])).into_string()));
            }
            if let Some(meta) = &trx.meta {
                for log in &meta.log_messages {
                    strings.push(format!("log:{}", log));
                }
            }
        }

        Ok(BenchOutput {
            count: strings.len() as u64,
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 3: Large allocation with realloc
#[no_mangle]
pub extern "C" fn bench_large_allocs_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<BenchOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut data = Vec::new();

        for trx in &blk.transactions {
            if let Some(transaction) = &trx.transaction {
                for sig in &transaction.signatures {
                    data.extend_from_slice(sig);
                }
            }
        }

        Ok(BenchOutput {
            count: data.len() as u64,
        })
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}

/// Benchmark 4: Mixed allocation pattern
#[no_mangle]
pub extern "C" fn bench_mixed_solana(blk_ptr: *mut u8, blk_len: usize) {
    substreams::register_panic_hook();
    let func = || -> Result<LargeOutput, Error> {
        let blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len)
            .expect("Unable to decode Block");

        let mut output = LargeOutput::default();
        // Similar mixed pattern for Solana transactions
        // ... implementation details

        Ok(output)
    };
    substreams::skip_empty_output();
    let result = func();
    if result.is_err() {
        panic!("{:?}", result.unwrap_err());
    }
    substreams::output(result.expect("already checked"));
}
```

#### Task 1.5: Create Makefile for building variants
- [ ] File: `wasm/bench/allocator_bench/Makefile`
- [ ] Content:
  ```makefile
  ALLOCATORS := default talc rlsf lol mini
  OUTPUT_DIR := ./wasm_variants

  .PHONY: all clean build-all

  all: build-all

  $(OUTPUT_DIR):
  	mkdir -p $(OUTPUT_DIR)

  # Build with default allocator (dlmalloc)
  build-default: $(OUTPUT_DIR)
  	cargo build --target wasm32-unknown-unknown --release
  	cp target/wasm32-unknown-unknown/release/substreams.wasm $(OUTPUT_DIR)/substreams-default.wasm
  	@echo "Built: $(OUTPUT_DIR)/substreams-default.wasm ($$(stat -f%z $(OUTPUT_DIR)/substreams-default.wasm 2>/dev/null || stat -c%s $(OUTPUT_DIR)/substreams-default.wasm) bytes)"

  # Build with Talc allocator
  build-talc: $(OUTPUT_DIR)
  	cargo build --target wasm32-unknown-unknown --release --features alloc-talc
  	cp target/wasm32-unknown-unknown/release/substreams.wasm $(OUTPUT_DIR)/substreams-talc.wasm
  	@echo "Built: $(OUTPUT_DIR)/substreams-talc.wasm"

  # Build with rlsf allocator
  build-rlsf: $(OUTPUT_DIR)
  	cargo build --target wasm32-unknown-unknown --release --features alloc-rlsf
  	cp target/wasm32-unknown-unknown/release/substreams.wasm $(OUTPUT_DIR)/substreams-rlsf.wasm
  	@echo "Built: $(OUTPUT_DIR)/substreams-rlsf.wasm"

  # Build with lol_alloc allocator
  build-lol: $(OUTPUT_DIR)
  	cargo build --target wasm32-unknown-unknown --release --features alloc-lol
  	cp target/wasm32-unknown-unknown/release/substreams.wasm $(OUTPUT_DIR)/substreams-lol.wasm
  	@echo "Built: $(OUTPUT_DIR)/substreams-lol.wasm"

  # Build with mini-alloc allocator
  build-mini: $(OUTPUT_DIR)
  	cargo build --target wasm32-unknown-unknown --release --features alloc-mini
  	cp target/wasm32-unknown-unknown/release/substreams.wasm $(OUTPUT_DIR)/substreams-mini.wasm
  	@echo "Built: $(OUTPUT_DIR)/substreams-mini.wasm"

  build-all: build-default build-talc build-rlsf build-lol build-mini
  	@echo "All allocator variants built in $(OUTPUT_DIR)/"
  	@ls -la $(OUTPUT_DIR)/*.wasm

  clean:
  	cargo clean
  	rm -rf $(OUTPUT_DIR)
  ```

#### Task 1.6: Create rust-toolchain.toml
- [ ] File: `wasm/bench/allocator_bench/rust-toolchain.toml`
- [ ] Content (use latest stable):
  ```toml
  [toolchain]
  channel = "1.93"
  components = ["rustfmt"]
  targets = ["wasm32-unknown-unknown"]
  ```

---

### Priority 2: Extend Go Benchmarks

#### Task 2.1: Create allocator benchmark test file
- [ ] File: `wasm/bench/allocator_bench_test.go`
- [ ] Location: Same directory as existing `bench_test.go`
- [ ] **IMPORTANT**: Use Go 1.24+ benchmark API with `b.Loop()` - no `b.ResetTimer()` needed
- [ ] **IMPORTANT**: Initialize VM/module OUTSIDE `b.Run` - only measure call throughput

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"

	_ "github.com/streamingfast/substreams/wasm/wasmtime"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func init() {
	logging.InstantiateLoggers()
}

// AllocatorVariant represents a WASM module built with a specific allocator
type AllocatorVariant struct {
	Name     string
	WasmPath string
	CodeSize int64
}

// TestBlock represents a test block from different chains
type TestBlock struct {
	Name           string
	Path           string
	InputType      string
	EntrypointSuffix string // Suffix for entrypoint names (e.g., "_ethereum", "_solana")
}

var testBlocks = []TestBlock{
	// Ethereum, BNB, Polygon all use sf.ethereum.type.v2.Block -> _ethereum entrypoints
	{"ethereum", "testdata/ethereum_mainnet_block_16021772.binpb", "sf.ethereum.type.v2.Block", "_ethereum"},
	{"bnb", "testdata/bnb_mainnet_block_78347457.binpb", "sf.ethereum.type.v2.Block", "_ethereum"},
	{"polygon", "testdata/polygon_mainnet_block_82340855.binpb", "sf.ethereum.type.v2.Block", "_ethereum"},
	// Solana uses sf.solana.type.v1.Block -> _solana entrypoints
	{"solana", "testdata/solana_mainnet_block_396976773.binpb", "sf.solana.type.v1.Block", "_solana"},
}

func loadAllocatorVariants(t testing.TB) []AllocatorVariant {
	variantsDir := "allocator_bench/wasm_variants"
	allocators := []string{"default", "talc", "rlsf", "lol", "mini"}

	var variants []AllocatorVariant
	for _, alloc := range allocators {
		path := filepath.Join(variantsDir, fmt.Sprintf("substreams-%s.wasm", alloc))
		info, err := os.Stat(path)
		if err != nil {
			t.Logf("Skipping %s: %v", alloc, err)
			continue
		}
		variants = append(variants, AllocatorVariant{
			Name:     alloc,
			WasmPath: path,
			CodeSize: info.Size(),
		})
	}
	return variants
}

func BenchmarkAllocators(b *testing.B) {
	// Base benchmark names (will be suffixed with chain, e.g., bench_decode_only_ethereum)
	benchmarkBases := []string{
		"decode_only",
		"small_allocs",
		"large_allocs",
		"mixed",
	}

	ctx := context.Background()
	os.Setenv("SUBSTREAMS_WASM_RUNTIME", "wasmtime")

	variants := loadAllocatorVariants(b)
	if len(variants) == 0 {
		b.Skip("No allocator variants found. Run 'make build-all' in allocator_bench/ first.")
	}

	for _, variant := range variants {
		wasmCode := readCode(b, variant.WasmPath)
		b.Logf("Allocator %s: WASM size = %d bytes", variant.Name, variant.CodeSize)

		for _, block := range testBlocks {
			blockData := readCode(b, block.Path)
			input := wasm.NewSourceInput(block.InputType, 0)

			for _, benchBase := range benchmarkBases {
				// Entrypoint is bench_{base}_{chain}, e.g., bench_decode_only_ethereum
				entrypoint := fmt.Sprintf("bench_%s%s", benchBase, block.EntrypointSuffix)
				name := fmt.Sprintf("alloc=%s/chain=%s/bench=%s", variant.Name, block.Name, benchBase)

				b.Run(name, func(b *testing.B) {
					// Setup: Initialize module and instance BEFORE the benchmark loop
					wasmRuntime := wasm.NewRegistry(nil)
					stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())

					module, err := wasmRuntime.NewModule(ctx, wasmCode, "wasm/rust-v1")
					require.NoError(b, err)
					defer module.Close(ctx)

					instance, err := module.NewInstance(ctx)
					require.NoError(b, err)
					defer instance.Close(ctx)

					call := wasm.NewCall(
						ctx,
						&pbsubstreams.Clock{Id: "bench", Number: 0},
						benchBase,
						entrypoint,
						stats,
						[]wasm.Argument{input},
						true,
						nil,
					)

					inputData := map[string][]byte{input.Name(): blockData}

					b.ReportAllocs()

					// Go 1.24+ benchmark API: b.Loop() handles iteration
					// No b.ResetTimer() needed - setup above is excluded automatically
					for b.Loop() {
						_, err := module.ExecuteNewCall(
							ctx, call, instance,
							[]wasm.Argument{input},
							inputData,
						)
						require.NoError(b, err)
					}
				})
			}
		}
	}
}

// BenchmarkAllocatorModuleSize reports WASM module sizes
func BenchmarkAllocatorModuleSize(b *testing.B) {
	variants := loadAllocatorVariants(b)

	for _, v := range variants {
		b.Run(fmt.Sprintf("alloc=%s", v.Name), func(b *testing.B) {
			b.ReportMetric(float64(v.CodeSize), "wasm_bytes")
		})
	}
}
```

#### Task 2.2: Add memory usage tracking
- [ ] Enhance benchmark to report peak WASM memory
- [ ] WASMTime provides `Memory.Size()` - access via:
  ```go
  // In wasm/wasmtime/instance.go, expose memory size
  func (i *instance) MemorySize() uint32 {
      return i.wasmInstance.GetExport(i.wasmStore, "memory").Memory().Size(i.wasmStore)
  }
  ```
- [ ] Use in benchmarks to report `b.ReportMetric(float64(memSize*65536), "peak_memory_bytes")`

---

### Priority 3: Benchmark Execution and Analysis

#### Task 3.1: Build all allocator variants
- [ ] Build all variants: `cd wasm/bench/allocator_bench && make build-all`
- [ ] Verify output: `ls -la wasm/bench/allocator_bench/wasm_variants/`

#### Task 3.2: Run benchmarks
- [ ] Command: `go test -bench=BenchmarkAllocators -benchmem -count=5 ./wasm/bench/...`
- [ ] Save output: `go test -bench=BenchmarkAllocators -benchmem -count=5 ./wasm/bench/... > benchmark_results.txt`

#### Task 3.3: Analyze results
- [ ] Compare execution time across allocators
- [ ] Compare memory allocations (Go side, from `-benchmem`)
- [ ] Compare WASM module sizes
- [ ] Note any allocator-specific issues or quirks

---

### Priority 4: Documentation and Recommendations

#### Task 4.1: Document findings
- [ ] Create `wasm/bench/allocator_bench/RESULTS.md` with:
  - Benchmark methodology
  - Raw results table
  - Analysis and conclusions
  - Recommendations

#### Task 4.2: Update substreams-rs if needed
- [ ] If a non-default allocator wins significantly, consider:
  - Adding allocator feature flags to `substreams` crate
  - Documenting the option for users
  - Potentially changing the default

---

## Answered Open Questions

### Q1: What's the typical protobuf message size in production Substreams?

**Answer**: Based on test data in the codebase:
- Ethereum: `testdata/ethereum_mainnet_block_16021772.binpb`
- Solana: `testdata/solana_mainnet_block_396976773.binpb` (heavy block)
- BNB: `testdata/bnb_mainnet_block_78347457.binpb` (heavy block)
- Polygon: `testdata/polygon_mainnet_block_82340855.binpb` (heavy block)
- Production ranges from small blocks (~10-50KB) to large blocks (several MB) depending on network activity

### Q2: Are there existing benchmark modules in the substreams engine we can extend?

**Answer**: Yes! The existing benchmark infrastructure is at:
- `wasm/bench/bench_test.go` - Main Go benchmark file with WASMTime integration
- `wasm/bench/substreams_wasm/` - Existing Rust benchmark module with handlers:
  - `map_decode_proto_only` - Protobuf decode benchmark
  - `map_block` - Full block processing benchmark
- Pattern: Use `wasm.NewRegistry()`, `module.NewInstance()`, `module.ExecuteNewCall()`

### Q3: Should we also measure compilation time (Rust -> WASM)?

**Answer**: Not in the primary benchmarks. Rationale:
- Compilation is a one-time cost during development
- Runtime performance is the primary concern for Substreams execution
- WASM module size (which affects load time) IS measured
- If needed, can add as secondary metric via `time cargo build ...`

### Q4: Is WASM SIMD enabled? Some allocators may benefit from it.

**Answer**: Based on `wasm/wasmtime/module.go`:
- WASMTime config uses default settings
- SIMD is NOT explicitly enabled or disabled
- For allocators, SIMD is generally not relevant (allocators don't use SIMD instructions)
- Could be investigated if allocator benchmarks show unexpected results

---

## Technical Notes

### How the WASM runtime works

1. **Module loading** (`wasm/wasmtime/module.go`):
   - Creates WASMTime engine with default config
   - Compiles WASM bytes to module
   - One module per user request/stream

2. **Instance creation** (`wasm/wasmtime/module.go:newInstance`):
   - Creates linker and store
   - Registers host imports (`env`, `logger`, `state` namespaces)
   - Instantiates WASM module
   - Gets `alloc` and `dealloc` exports from WASM

3. **Heap management** (`wasm/wasmtime/heap.go`):
   - Calls WASM's exported `alloc(size)` to allocate memory
   - Tracks allocations for cleanup via `dealloc(ptr, len)`
   - Uses `memory.UnsafeData()` for direct memory access

### Required WASM exports

Any Substreams WASM module must export:
- `alloc(size: i32) -> i32` - Allocate memory, return pointer
- `dealloc(ptr: i32, len: i32)` - Deallocate memory
- `memory` - The linear memory
- Handler functions (e.g., `map_block`)

These are provided by the `substreams` Rust crate.

### Test data (multiple chains)

- `wasm/bench/testdata/ethereum_mainnet_block_16021772.binpb` - Ethereum mainnet block
- `wasm/bench/testdata/solana_mainnet_block_396976773.binpb` - Solana mainnet block (heavy)
- `wasm/bench/testdata/bnb_mainnet_block_78347457.binpb` - BNB mainnet block (heavy)
- `wasm/bench/testdata/polygon_mainnet_block_82340855.binpb` - Polygon mainnet block (heavy)

These cover different allocation patterns and block sizes across multiple chains.

---

## References

- [Talc GitHub](https://github.com/SFBdragon/talc) - includes WASM benchmarks
- [Talc WASM README](https://github.com/SFBdragon/talc/blob/master/talc/README_WASM.md)
- [rlsf Documentation](https://docs.rs/rlsf)
- [Avoiding allocations in Rust for WASM](https://nickb.dev/blog/avoiding-allocations-in-rust-to-shrink-wasm-modules/)
- [Hacker News: Talc discussion](https://news.ycombinator.com/item?id=39545574)
- Existing benchmark: `wasm/bench/bench_test.go`
- Existing benchmark module: `wasm/bench/substreams_wasm/`
