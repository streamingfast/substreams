package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"

	_ "github.com/streamingfast/substreams/wasm/wasmtime"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type copyDecodeFixture struct {
	chain string
	full  string
	light string
}

var copyDecodeFixtures = []copyDecodeFixture{
	{"ethereum", "testdata/ethereum_mainnet_block_16021772.binpb", "testdata/ethereum_mainnet_block_16021772_light.binpb"},
	{"bnb", "testdata/bnb_mainnet_block_78347457.binpb", "testdata/bnb_mainnet_block_78347457_light.binpb"},
	{"polygon", "testdata/polygon_mainnet_block_82340855.binpb", "testdata/polygon_mainnet_block_82340855_light.binpb"},
}

type copyDecodeDecoder struct {
	name       string
	wasmPath   string
	entrypoint string
}

// copy_only uses map_noop: the host still writes the full block into guest
// linear memory (ExecuteNewCall does that unconditionally for any SourceInput
// argument, regardless of what the entrypoint does with it), but the guest
// does no decoding. It isolates the host->guest copy cost; subtracting it
// from a decode_* number estimates that decoder's decode+iterate cost alone.
var copyDecodeDecoders = []copyDecodeDecoder{
	{"copy_only", "substreams_wasm/substreams.wasm", "map_noop"},
	{"decode_prost", "substreams_wasm/substreams.wasm", "map_count_logs"},
	{"decode_buffa_eager", "substreams_wasm_buffa/substreams.wasm", "map_count_logs_eager"},
	{"decode_buffa_lazy", "substreams_wasm_buffa/substreams.wasm", "map_count_logs_lazy"},
}

// BenchmarkCopyDecode measures, for full vs light EVM blocks, the steady-state
// cost of the wasmtime host->guest path: copying a block into WASM linear
// memory alone (copy_only), versus copying and then decoding+iterating all
// receipt logs (decode_*). The instance is created once and reused across
// b.Loop() iterations, so per-iteration cost excludes module compilation and
// instantiation.
func BenchmarkCopyDecode(b *testing.B) {
	ctx := context.Background()
	os.Setenv("SUBSTREAMS_WASM_RUNTIME", "wasmtime")

	wasmCodeCache := map[string][]byte{}
	loadCode := func(path string) []byte {
		if code, ok := wasmCodeCache[path]; ok {
			return code
		}
		code := readCode(b, path)
		wasmCodeCache[path] = code
		return code
	}

	for _, fixture := range copyDecodeFixtures {
		for _, variant := range []struct {
			name string
			path string
		}{
			{"full", fixture.full},
			{"light", fixture.light},
		} {
			blockData := readCode(b, variant.path)
			input := wasm.NewSourceInput("sf.ethereum.type.v2.Block", 0)
			inputData := map[string][]byte{input.Name(): blockData}

			for _, decoder := range copyDecodeDecoders {
				name := fmt.Sprintf("chain=%s,block=%s,decoder=%s", fixture.chain, variant.name, decoder.name)

				b.Run(name, func(b *testing.B) {
					wasmRuntime := wasm.NewRegistry(nil)
					stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())

					module, err := wasmRuntime.NewModule(ctx, loadCode(decoder.wasmPath), "wasm/rust-v1")
					require.NoError(b, err)
					defer module.Close(ctx)

					instance, err := module.NewInstance(ctx)
					require.NoError(b, err)
					defer instance.Close(ctx)

					call := wasm.NewCall(
						ctx,
						&pbsubstreams.Clock{Id: "bench", Number: 0},
						decoder.name,
						decoder.entrypoint,
						stats,
						[]wasm.Argument{input},
						true,
						nil,
					)

					b.SetBytes(int64(len(blockData)))
					b.ReportAllocs()

					for b.Loop() {
						_, err := module.ExecuteNewCall(ctx, call, instance, []wasm.Argument{input}, inputData)
						require.NoError(b, err)

						// Matches the per-call lifecycle in pipeline/exec/baseexec.go: production
						// clears heap allocations between calls on a reused instance. Skipping this
						// leaks every write and OOMs the guest allocator within a few thousand iterations.
						require.NoError(b, instance.Cleanup(ctx))
					}
				})
			}
		}
	}
}

// runLogCountDecoder executes one decode_* entrypoint once through the real
// wasmtime path and returns the log count it wrote out via output_raw
// (little-endian u64).
func runLogCountDecoder(t require.TestingT, decoder copyDecodeDecoder, blockData []byte) uint64 {
	ctx := context.Background()
	os.Setenv("SUBSTREAMS_WASM_RUNTIME", "wasmtime")

	wasmRuntime := wasm.NewRegistry(nil)
	stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())

	module, err := wasmRuntime.NewModule(ctx, readCode(t, decoder.wasmPath), "wasm/rust-v1")
	require.NoError(t, err)
	defer module.Close(ctx)

	instance, err := module.NewInstance(ctx)
	require.NoError(t, err)
	defer instance.Close(ctx)

	input := wasm.NewSourceInput("sf.ethereum.type.v2.Block", 0)
	call := wasm.NewCall(
		ctx,
		&pbsubstreams.Clock{Id: "verify", Number: 0},
		decoder.name,
		decoder.entrypoint,
		stats,
		[]wasm.Argument{input},
		true,
		nil,
	)

	_, err = module.ExecuteNewCall(ctx, call, instance, []wasm.Argument{input}, map[string][]byte{input.Name(): blockData})
	require.NoError(t, err)

	output := call.Output()
	require.Len(t, output, 8, "expected an 8-byte little-endian u64 log count")

	return binary.LittleEndian.Uint64(output)
}

// TestCopyDecodeLogCountsAgree exercises map_count_logs (prost),
// map_count_logs_eager and map_count_logs_lazy through the exact same
// wasmtime path and .wasm artifacts used by BenchmarkCopyDecode, and asserts
// all three report the identical, non-zero log count for every fixture. This
// guards against the lazy entrypoint being fast because it silently skips
// decoding real log data rather than because it defers unrelated work.
func TestCopyDecodeLogCountsAgree(t *testing.T) {
	logCountDecoders := []copyDecodeDecoder{
		{"decode_prost", "substreams_wasm/substreams.wasm", "map_count_logs"},
		{"decode_buffa_eager", "substreams_wasm_buffa/substreams.wasm", "map_count_logs_eager"},
		{"decode_buffa_lazy", "substreams_wasm_buffa/substreams.wasm", "map_count_logs_lazy"},
	}

	for _, fixture := range copyDecodeFixtures {
		for _, variant := range []struct {
			name string
			path string
		}{
			{"full", fixture.full},
			{"light", fixture.light},
		} {
			t.Run(fmt.Sprintf("chain=%s,block=%s", fixture.chain, variant.name), func(t *testing.T) {
				blockData := readCode(t, variant.path)

				var reference uint64
				for i, decoder := range logCountDecoders {
					count := runLogCountDecoder(t, decoder, blockData)
					require.NotZero(t, count, "decoder %s reported zero logs, fixture has none or decode silently no-op'd", decoder.name)

					if i == 0 {
						reference = count
						continue
					}
					require.Equal(t, reference, count, "decoder %s disagrees with %s on log count", decoder.name, logCountDecoders[0].name)
				}
			})
		}
	}
}
