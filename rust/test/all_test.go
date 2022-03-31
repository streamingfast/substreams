package test

//go:generate ./build.sh

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"

	"github.com/streamingfast/bstream"
	imports "github.com/streamingfast/substreams/native-imports"
	"github.com/streamingfast/substreams/pipeline"

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
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, "bigint", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("test.key.1")
				require.True(t, found)
				require.Equal(t, "20", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_sum_int64",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, "int64", nil),
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
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, "float64", nil),
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
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, "bigFloat", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("sum.big.float")
				require.True(t, found)
				require.Equal(t, "21", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_sum_big_float_big_number",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_SUM, "bigFloat", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("sum.big.float")
				require.True(t, found)
				require.Equal(t, "24691357975308643", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_min_int64",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, "int64", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_min_int64")
				require.True(t, found)
				require.Equal(t, "2", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_min_bigint",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, "bigInt", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_min_bigint")
				require.True(t, found)
				require.Equal(t, "3", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_min_float64",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, "float64", nil),
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
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MIN, "bigFloat", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_min_bigfloat")
				require.True(t, found)
				require.Equal(t, "11.04", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_max_int64",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, "int64", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_max_int64")
				require.True(t, found)
				require.Equal(t, "5", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_max_bigint",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, "bigInt", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_max_bigint")
				require.True(t, found)
				require.Equal(t, "5", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_max_float64",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, "float64", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_max_float64")
				require.True(t, found)
				actual, err := strconv.ParseFloat(string(data), 100)
				require.NoError(t, err)
				require.Equal(t, 10.05, actual)
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_max_bigfloat",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_MAX, "bigFloat", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				data, found := builder.GetLast("set_max_bigfloat")
				require.True(t, found)
				require.Equal(t, "11.05", string(data))
			},
		},
		{
			wasmFile:     "./target/wasm32-unknown-unknown/release/testing_substreams.wasm",
			functionName: "test_set_delete_prefix",
			builder:      state.NewBuilder("builder.name.1", 0, pbtransform.KindStore_UPDATE_POLICY_IGNORE, "some object", nil),
			assert: func(t *testing.T, module *wasm.Module, instance *wasm.Instance, builder *state.Builder) {
				_, found := builder.GetLast("1:key_to_keep")
				require.True(t, found, "key_to_keep")
				_, found = builder.GetLast("2:key_to_delete")
				require.False(t, found, "key_to_delete")
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

			imps := &imports.Imports{}
			imps.SetCurrentBlock(bstream.NewBlockRef("test", 42))

			instance, err := module.NewInstance(c.functionName, nil, pipeline.GetRPCWasmFunctionFactory(nil))
			require.NoError(t, err)
			instance.SetOutputStore(c.builder)
			err = instance.Execute()
			require.NoError(t, err)
			c.assert(t, module, instance, c.builder)
		})
	}
}

func Test_MakeItCrash(t *testing.T) {
	t.Skip()
	file, err := os.Open("./target/wasm32-unknown-unknown/release/testing_substreams.wasm")
	require.NoError(t, err)
	byteCode, err := ioutil.ReadAll(file)
	require.NoError(t, err)

	//done := make(chan interface{})

	//mutex := sync.Mutex{}
	wg := sync.WaitGroup{}
	data := make([]byte, (1024*1024)*1)
	module, err := wasm.NewModule(byteCode, "test_make_it_crash")
	require.NoError(t, err)
	for i := 0; i < 100; i++ {
		fmt.Println("iteration:", i)
		start := time.Now()
		for j := 0; j < 100; j++ {
			wg.Add(1)
			go func(id int) {
				//fmt.Print(id, "-")
				//runtime.LockOSThread()

				instance, err := module.NewInstance("test_make_it_crash", nil, pipeline.GetRPCWasmFunctionFactory(nil))
				time.Sleep(10 * time.Millisecond)
				ptr, err := instance.Heap().Write(data)

				require.NoError(t, err)
				err = instance.ExecuteWithArgs(ptr, int32(len(data)))

				//mutex.Unlock()

				require.NoError(t, err)
				require.Equal(t, len(data), len(instance.Output()))
				wg.Done()
			}(j)
		}
		//close(done)

		fmt.Println("waiting")
		//<-done
		wg.Wait()
		//runtime.GC()
		//time.Sleep(10 * time.Millisecond)
		fmt.Println("done:", time.Since(start))
	}
	////close(done)
	//
	//fmt.Println("waiting")
	////<-done
	//wg.Wait()
	//fmt.Println("done")
}
