package main

import (
	"sync"
	"testing"

	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"

	_ "github.com/streamingfast/substreams/wasm/wasmtime"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestConcurrentInstancesFromSharedModule compiles one wasmtime module once and
// instantiates + executes it from many goroutines concurrently, run under -race,
// to check whether sharing a single compiled wasmtime.Module/Engine across
// concurrently-running tier2 requests is safe.
func TestConcurrentInstancesFromSharedModule(t *testing.T) {
	ctx := t.Context()
	t.Setenv("SUBSTREAMS_WASM_RUNTIME", "wasmtime")

	code := readCode(t, "substreams_wasm/substreams.wasm")
	registry := wasm.NewRegistry(nil)
	module, err := registry.NewModule(ctx, code, "wasm/rust-v1")
	require.NoError(t, err)

	input := blockInputFile(t, "testdata/ethereum_mainnet_block_16021772.binpb")
	stats := metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop())

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for g := range goroutines {
		wg.Go(func() {
			instance, err := module.NewInstance(ctx)
			if err != nil {
				errs[g] = err
				return
			}
			defer instance.Close(ctx)

			call := wasm.NewCall(ctx, &pbsubstreams.Clock{Id: "a", Number: 0}, "concurrency_probe", "map_decode_proto_only", stats, []wasm.Argument{input.arg}, true, nil)
			_, err = module.ExecuteNewCall(ctx, call, instance, []wasm.Argument{input.arg}, map[string][]byte{input.arg.Name(): input.val})
			errs[g] = err
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
}
