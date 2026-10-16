package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/wasm"

	_ "github.com/streamingfast/substreams/wasm/wasmtime"
	_ "github.com/streamingfast/substreams/wasm/wazero"

	"github.com/stretchr/testify/require"
)

type compileBinary struct {
	tag      string
	code     []byte
	codeType string
}

// BenchmarkCompile isolates the cost of compiling a WASM binary (wasmtime.NewModule
// or wazero.CompileModule), excluding instantiation and execution, across binaries
// of different size and across runtimes.
func BenchmarkCompile(b *testing.B) {
	bins := []compileBinary{
		{"bench_substreams.wasm", readCode(b, "substreams_wasm/substreams.wasm"), "wasm/rust-v1"},
	}
	bins = append(bins, spkgBinaries(b, "big_substreams", "../../test/testdata/big_substreams/big-substreams-v0.1.0.spkg")...)
	bins = append(bins, spkgBinaries(b, "wasmbindgen_substreams", "../../test/testdata/wasmbindgen_substreams/wasmbindgen-substreams-v0.1.0.spkg")...)

	for _, runtimeName := range []string{"wasmtime", "wazero"} {
		for _, bin := range bins {
			b.Run(fmt.Sprintf("vm=%s,bin=%s,bytes=%d", runtimeName, bin.tag, len(bin.code)), func(b *testing.B) {
				b.Setenv("SUBSTREAMS_WASM_RUNTIME", runtimeName)
				ctx := context.Background()

				b.ReportAllocs()
				for b.Loop() {
					registry := wasm.NewRegistry(nil)
					_, err := registry.NewModule(ctx, bin.code, bin.codeType)
					require.NoError(b, err)
				}
			})
		}
	}
}

func spkgBinaries(t require.TestingT, tagPrefix, path string) []compileBinary {
	bundle, err := manifest.MustNewReader(path).Read()
	require.NoError(t, err)

	out := make([]compileBinary, 0, len(bundle.Package.Modules.Binaries))
	for i, bin := range bundle.Package.Modules.Binaries {
		out = append(out, compileBinary{
			tag:      fmt.Sprintf("%s_%d", tagPrefix, i),
			code:     bin.Content,
			codeType: bin.Type,
		})
	}
	return out
}
