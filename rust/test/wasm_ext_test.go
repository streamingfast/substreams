package test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/streamingfast/substreams/wasm"
	"github.com/test-go/testify/assert"
	"github.com/test-go/testify/require"
)

func TestExtensionCalls(t *testing.T) {
	cases := []struct {
		functionName string
		expectError  error
		expectLogs   []string
	}{
		{
			functionName: "test_wasm_extension_hello",
			expectLogs:   []string{"first", "second"},
		},
		{
			functionName: "test_wasm_extension_fail",
			expectError:  errors.New(`executing entrypoint "test_wasm_extension_fail": failed running wasm extension "myext::myimport": expected hello`),
			expectLogs:   []string{"first"},
		},
	}
	for _, c := range cases {
		t.Run(c.functionName, func(t *testing.T) {
			file, err := os.Open("./target/wasm32-unknown-unknown/release/testing_substreams.wasm")
			require.NoError(t, err)
			byteCode, err := ioutil.ReadAll(file)
			require.NoError(t, err)

			rpcProv := &testWasmExtension{}
			runtime := wasm.NewRuntime([]wasm.WASMExtensioner{rpcProv})
			module, err := runtime.NewModule(byteCode, c.functionName)
			require.NoError(t, err)

			instance, err := module.NewInstance(c.functionName, nil)
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
		"myext": map[string]wasm.WASMExtension{
			"myimport": func(in []byte) (out []byte, err error) {
				i.called = true
				if string(in) == "hello" {
					return []byte("world"), nil
				}
				i.errored = true
				return nil, fmt.Errorf("expected hello")
			},
		},
	}
}
