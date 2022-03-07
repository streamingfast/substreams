package test

//go:generate ./build.sh

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"github.com/test-go/testify/require"
)

func TestRustScript(t *testing.T) {
	cases := []struct {
		wasmFile     string
		functionName string
		parameters   []interface{}
		builder      *state.Builder
		assert       func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder)
	}{
		{
			functionName: "test_sum_big_int",
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			builder:      state.NewBuilder("builder.name.1", "sum", "bigint", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("test.key.1")
				require.True(t, found)
				require.Equal(t, "20", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_sum_int64",
			builder:      state.NewBuilder("builder.name.1", "sum", "int64", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("sum.int.64")
				require.True(t, found)
				val, _ := strconv.ParseInt(string(data), 10, 64)
				require.Equal(t, int64(20), val)
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_sum_float64",
			builder:      state.NewBuilder("builder.name.1", "sum", "float64", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("sum.float.64")
				require.True(t, found)
				val, _ := strconv.ParseFloat(string(data), 64)
				require.Equal(t, 21.5, val)
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_sum_big_float_small_number",
			builder:      state.NewBuilder("builder.name.1", "sum", "bigFloat", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("sum.big.float")
				require.True(t, found)
				require.Equal(t, "21", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_sum_big_float_big_number",
			builder:      state.NewBuilder("builder.name.1", "sum", "bigFloat", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("sum.big.float")
				require.True(t, found)
				require.Equal(t, "24691357975308643", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_min_int64",
			builder:      state.NewBuilder("builder.name.1", "min", "int64", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_min_int64")
				require.True(t, found)
				require.Equal(t, "2", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_min_bigint",
			builder:      state.NewBuilder("builder.name.1", "min", "bigInt", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_min_bigint")
				require.True(t, found)
				require.Equal(t, "3", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_min_float64",
			builder:      state.NewBuilder("builder.name.1", "min", "float64", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_min_float64")
				require.True(t, found)
				v, err := strconv.ParseFloat(string(data), 100)
				require.NoError(t, err)
				require.Equal(t, 10.04, v)
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_min_bigfloat",
			builder:      state.NewBuilder("builder.name.1", "min", "bigFloat", "", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_min_bigfloat")
				require.True(t, found)
				require.Equal(t, "11.04", string(data))
			},
		},
	}

	for _, c := range cases {
		t.Run(c.functionName, func(t *testing.T) {
			file, err := os.Open(c.wasmFile)
			require.NoError(t, err)
			byteCode, err := ioutil.ReadAll(file)
			require.NoError(t, err)
			module, err := wasm.NewModule(byteCode, c.functionName)
			require.NoError(t, err)

			instance, err := module.NewInstance(c.functionName, nil)
			require.NoError(t, err)
			instance.SetOutputStore(c.builder)
			err = instance.Execute()
			require.NoError(t, err)

			c.assert(t, module, instance, c.builder)
		})
	}
}
