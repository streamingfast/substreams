package wazero

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"

	"github.com/streamingfast/substreams/wasm"
)

var stateFuncs = []funcs{
	{
		"set",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readBytesFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSet(ord, key, value)
		}),
	},
	{
		"set_if_not_exists",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readBytesFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetIfNotExists(ord, key, value)
		}),
	},
	{
		"append",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readBytesFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoAppend(ord, key, value)
		}),
	},
	{
		"delete_prefix",
		[]parm{i64, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			prefix := readStringFromStack(mod, stack[1:])
			call := wasm.FromContext(ctx)

			call.DoDeletePrefix(ord, prefix)
		}),
	},
	{
		"add_bigint",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoAddBigInt(ord, key, value)
		}),
	},
	{
		"add_bigdecimal",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoAddBigDecimal(ord, key, value)
		}),
	},
	{
		"add_int64",
		[]parm{i64, i32, i32, i64},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := int64(stack[3])
			call := wasm.FromContext(ctx)

			call.DoAddInt64(ord, key, value)
		}),
	},
	{
		"add_float64",
		[]parm{i64, i32, i32, f64},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := api.DecodeF64(stack[3])
			call := wasm.FromContext(ctx)

			call.DoAddFloat64(ord, key, value)
		}),
	},
	{
		"set_min_int64",
		[]parm{i64, i32, i32, i64},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := int64(stack[3])
			call := wasm.FromContext(ctx)

			call.DoSetMinInt64(ord, key, value)
		}),
	},
	{
		"set_min_bigint",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetMinBigInt(ord, key, value)
		}),
	},
	{
		"set_min_float64",
		[]parm{i64, i32, i32, f64},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := api.DecodeF64(stack[3])
			call := wasm.FromContext(ctx)

			call.DoSetMinFloat64(ord, key, value)
		}),
	},
	{
		"set_min_bigdecimal",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetMinBigDecimal(ord, key, value)
		}),
	},

	{
		"set_max_int64",
		[]parm{i64, i32, i32, i64},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := int64(stack[3])
			call := wasm.FromContext(ctx)

			call.DoSetMaxInt64(ord, key, value)
		}),
	},
	{
		"set_max_bigint",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetMaxBigInt(ord, key, value)
		}),
	},
	{
		"set_max_float64",
		[]parm{i64, i32, i32, f64},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := api.DecodeF64(stack[3])
			call := wasm.FromContext(ctx)

			call.DoSetMaxFloat64(ord, key, value)
		}),
	},
	{
		"set_max_bigdecimal",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetMaxBigDecimal(ord, key, value)
		}),
	},

	// Getter functions

	{
		"get_at",
		[]parm{i32, i64, i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			ord := stack[1]
			key := readStringFromStack(mod, stack[2:])
			outputPtr := uint32(stack[4])
			call := wasm.FromContext(ctx)

			value, found := call.DoGetAt(int(storeIndex), ord, key)
			if !found {
				stack[0] = 0
			} else {
				if err := writeOutputToHeap(ctx, mod, outputPtr, value); err != nil {
					call.ReturnError(fmt.Errorf("writing output to heap: %w", err))
				}
				stack[0] = 1
			}
		}),
	},
	{
		"has_at",
		[]parm{i32, i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			ord := stack[1]
			key := readStringFromStack(mod, stack[2:])
			call := wasm.FromContext(ctx)

			found := call.DoHasAt(int(storeIndex), ord, key)
			if !found {
				stack[0] = 0
			} else {
				stack[0] = 1
			}
		}),
	},
	{
		"get_first",
		[]parm{i32, i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			key := readStringFromStack(mod, stack[1:])
			outputPtr := uint32(stack[3])
			call := wasm.FromContext(ctx)

			value, found := call.DoGetFirst(int(storeIndex), key)
			if !found {
				stack[0] = 0
			} else {
				if err := writeOutputToHeap(ctx, mod, outputPtr, value); err != nil {
					call.ReturnError(fmt.Errorf("writing output to heap: %w", err))
				}
				stack[0] = 1
			}
		}),
	},
	{
		"has_first",
		[]parm{i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			key := readStringFromStack(mod, stack[1:])
			call := wasm.FromContext(ctx)

			found := call.DoHasFirst(int(storeIndex), key)
			if !found {
				stack[0] = 0
			} else {
				stack[0] = 1
			}
		}),
	},
	{
		"get_last",
		[]parm{i32, i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			key := readStringFromStack(mod, stack[1:])
			outputPtr := uint32(stack[3])
			call := wasm.FromContext(ctx)

			value, found := call.DoGetLast(int(storeIndex), key)
			if !found {
				stack[0] = 0
			} else {
				if err := writeOutputToHeap(ctx, mod, outputPtr, value); err != nil {
					call.ReturnError(fmt.Errorf("writing output to heap: %w", err))
				}
				stack[0] = 1
			}
		}),
	},
	{
		"has_last",
		[]parm{i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			key := readStringFromStack(mod, stack[1:])
			call := wasm.FromContext(ctx)

			found := call.DoHasLast(int(storeIndex), key)
			if !found {
				stack[0] = 0
			} else {
				stack[0] = 1
			}
		}),
	},
}
