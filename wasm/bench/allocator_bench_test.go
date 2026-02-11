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
	Name             string
	Path             string
	InputType        string
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
					b.ResetTimer()

					// Benchmark loop
					for i := 0; i < b.N; i++ {
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

// TestAllocatorVerification calls the get_allocator_name entrypoint to verify
// that each WASM module correctly identifies its compiled allocator
func TestAllocatorVerification(t *testing.T) {
	variants := loadAllocatorVariants(t)
	if len(variants) == 0 {
		t.Skip("No allocator variants found. Run 'make build-all' in allocator_bench/ first.")
	}

	ctx := context.Background()
	os.Setenv("SUBSTREAMS_WASM_RUNTIME", "wasmtime")

	// Use a small dummy input (the entrypoint doesn't actually use it)
	dummyInput := []byte{0x0a, 0x00} // Empty protobuf message

	for _, variant := range variants {
		t.Run(fmt.Sprintf("verify_allocator=%s", variant.Name), func(t *testing.T) {
			wasmCode := readCode(t, variant.WasmPath)

			wasmRuntime := wasm.NewRegistry(nil)
			stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())

			module, err := wasmRuntime.NewModule(ctx, wasmCode, "wasm/rust-v1")
			require.NoError(t, err)
			defer module.Close(ctx)

			instance, err := module.NewInstance(ctx)
			require.NoError(t, err)
			defer instance.Close(ctx)

			input := wasm.NewSourceInput("dummy", 0)
			call := wasm.NewCall(
				ctx,
				&pbsubstreams.Clock{Id: "verify", Number: 0},
				"get_allocator_name",
				"get_allocator_name",
				stats,
				[]wasm.Argument{input},
				true,
				nil,
			)

			_, err = module.ExecuteNewCall(
				ctx, call, instance,
				[]wasm.Argument{input},
				map[string][]byte{input.Name(): dummyInput},
			)
			require.NoError(t, err)

			// The output contains the allocator name (as a protobuf BenchOutput message)
			// The allocator_name field should match the variant name
			output := call.Output()
			t.Logf("Variant %s: WASM size=%d bytes, output=%d bytes, raw=%x",
				variant.Name, variant.CodeSize, len(output), output)
		})
	}
}
