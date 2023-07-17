package main

import (
	"errors"
	"os"

	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/wasm"
	_ "github.com/streamingfast/substreams/wasm/goja"
	_ "github.com/streamingfast/substreams/wasm/wasmtime"
	_ "github.com/streamingfast/substreams/wasm/wazero"
	"github.com/stretchr/testify/require"
)

func init() {
	logging.InstantiateLoggers()
}

func BenchmarkExecution(b *testing.B) {
	type runtime struct {
		name                string
		code                []byte
		shouldReUseInstance bool
	}

	type testCase struct {
		tag        string
		entrypoint string
		arguments  []wasm.Argument
		// Right now there is differences between runtime, so we accept all those values
		acceptedByteCount []int
	}

	for _, testCase := range []*testCase{
		{"bare", "map_noop", args(wasm.NewParamsInput("")), []int{0}},

		// Decode proto only decode and returns the block.number as the output (to ensure the block is not elided at compile time)
		{"decode_proto_only", "map_decode_proto_only", args(blockInputFile(b, "testdata/ethereum_mainnet_block_16021772.binpb")), []int{0}},

		{"map_block", "map_block", args(blockInputFile(b, "testdata/ethereum_mainnet_block_16021772.binpb")), []int{44957, 45081}},
	} {
		var reuseInstance = true
		var freshInstanceEachRun = false

		jsCode := readCode(b, "substreams_ts/index.js")
		wasmCode := readCode(b, "substreams_wasm/substreams.wasm")

		for _, config := range []*runtime{
			{"wasmtime", wasmCode, reuseInstance},
			{"wasmtime", wasmCode, freshInstanceEachRun},

			{"wazero", wasmCode, reuseInstance},
			{"wazero", wasmCode, freshInstanceEachRun},

			{"goja", jsCode, reuseInstance},
			{"goja", jsCode, freshInstanceEachRun},
		} {
			suffix := "_reuse_instance"
			if !config.shouldReUseInstance {
				suffix = "_fresh_instance"
			}

			b.Run(config.name+"_"+testCase.tag+suffix, func(b *testing.B) {
				ctx := context.Background()

				wasmRuntime := wasm.NewRegistryWithRuntime(config.name, nil, 0)

				module, err := wasmRuntime.NewModule(ctx, config.code)
				require.NoError(b, err)

				cachedInstance, err := module.NewInstance(ctx)
				require.NoError(b, err)
				defer cachedInstance.Close(ctx)

				call := wasm.NewCall(nil, testCase.tag, testCase.entrypoint, testCase.arguments)

				for i := 0; i < b.N; i++ {
					instance := cachedInstance
					if !config.shouldReUseInstance {
						instance, err = module.NewInstance(ctx)
						require.NoError(b, err)
					}

					_, err := module.ExecuteNewCall(ctx, call, instance, testCase.arguments)
					if err != nil {
						var ex *goja.Exception
						if errors.As(err, &ex) {
							require.NoError(b, err, ex.String())
						}

						require.NoError(b, err)
					}

					require.Contains(b, testCase.acceptedByteCount, len(call.Output()), "invalid byte count got %d expected one of %v", len(call.Output()), testCase.acceptedByteCount)
				}
			})
		}
	}
}

func readCode(t require.TestingT, filename string) []byte {
	content, err := os.ReadFile(filename)
	require.NoError(t, err)

	return content
}

func args(ins ...wasm.Argument) []wasm.Argument {
	return ins
}

func blockInputFile(t require.TestingT, filename string) wasm.Argument {
	content, err := os.ReadFile(filename)
	require.NoError(t, err)

	input := wasm.NewSourceInput("sf.ethereum.type.v2.Block")
	input.SetValue(content)

	return input
}
