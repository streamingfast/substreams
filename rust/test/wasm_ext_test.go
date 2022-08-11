package test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionCalls(t *testing.T) {
	cases := []struct {
		wasmFile     string
		functionName string
		expectError  error
		expectLogs   []string
	}{
		{
			wasmFile:     "testing_substreams.wasm",
			functionName: "test_wasm_extension_hello",
			expectLogs:   []string{"first", "second"},
		},
		//{ // bring back later
		//	wasmFile:     "testing_substreams.wasm",
		//	functionName: "test_wasm_extension_fail",
		//	expectError:  errors.New("executing entrypoint \"test_wasm_extension_fail\": running wasm extension \"myext::myimport\": expected hello (recovered by wasmtime)\nwasm stack trace:\n\tmyext.myimport(i32,i32,i32)\n\t._ZN18testing_substreams11do_myimport17hedb8a0f8ca1d7d8cE(i32,i32)\n\t.test_wasm_extension_fail()"),
		//	expectLogs:   []string{"first"},
		//},
	}
	for _, c := range cases {
		t.Run(c.functionName, func(t *testing.T) {
			wasmFilePath := test_wasm_path(t, c.wasmFile)
			file, err := os.Open(wasmFilePath)
			require.NoError(t, err)
			byteCode, err := ioutil.ReadAll(file)
			require.NoError(t, err)

			rpcProv := &testWasmExtension{}
			runtime := wasm.NewRuntime([]wasm.WASMExtensioner{rpcProv})

			module, err := runtime.NewModule(context.Background(), &pbsubstreams.Request{}, byteCode, "module.1", c.functionName)
			require.NoError(t, err)

			instance, err := module.NewInstance(&pbsubstreams.Clock{}, nil)
			require.NoError(t, err)

			err = instance.Execute()
			if c.expectError != nil {
				assert.Equal(t, c.expectError.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}

			assert.True(t, rpcProv.called)
			assert.Equal(t, c.expectLogs, instance.Logs)
		})
	}
}

type testWasmExtension struct {
	called  bool
	errored bool
}

func (i *testWasmExtension) WASMExtensions() map[string]map[string]wasm.WASMExtension {
	return map[string]map[string]wasm.WASMExtension{
		"myext": {
			"myimport": func(context context.Context, req *pbsubstreams.Request, clock *pbsubstreams.Clock, in []byte) (out []byte, err error) {
				i.called = true
				if string(in) == "hello" {
					return []byte("world"), nil
				}
				i.errored = true
				return nil, fmt.Errorf("got: %q, expected: hello", string(in))
			},
		},
	}
}
