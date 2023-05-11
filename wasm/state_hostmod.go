package wasm

import (
	"context"
	"fmt"
	"math/big"

	"github.com/tetratelabs/wazero/api"
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
			call := fromContext(ctx)

			call.validateSetStore(key)

			call.outputStore.SetBytes(ord, key, value)
			call.traceStateWrites("set", key)
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
			call := fromContext(ctx)

			call.validateSetIfNotExists(key)

			call.outputStore.SetBytesIfNotExists(ord, key, value)
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
			call := fromContext(ctx)

			call.validateAppend(key)

			if err := call.outputStore.Append(ord, key, value); err != nil {
				call.returnError(fmt.Errorf("appending to store: %w", err))
			}
		}),
	},
	{
		"delete_prefix",
		[]parm{i64, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			prefix := readStringFromStack(mod, stack[1:])
			call := fromContext(ctx)

			call.traceStateWrites("delete_prefix", prefix)

			call.outputStore.DeletePrefix(ord, prefix)
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
			call := fromContext(ctx)

			call.validateAddBigInt(key)

			toAdd, _ := new(big.Int).SetString(value, 10)
			call.outputStore.SumBigInt(ord, key, toAdd)
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
			call := fromContext(ctx)

			call.validateAddBigDecimal(key)

			toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigDecimal's read of the kv value
			if err != nil {
				call.returnError(fmt.Errorf("parsing bigdecimal: %w", err))
			}
			call.outputStore.SumBigDecimal(ord, key, toAdd)
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
			call := fromContext(ctx)

			call.validateAddInt64(key)

			call.outputStore.SumInt64(ord, key, value)
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
			call := fromContext(ctx)

			call.validateAddFloat64(key)

			call.outputStore.SumFloat64(ord, key, value)
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
			call := fromContext(ctx)

			call.validateSetMinInt64(key)

			call.outputStore.SetMinInt64(ord, key, value)
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
			call := fromContext(ctx)

			call.validateSetMinBigInt(key)

			toSet, _ := new(big.Int).SetString(value, 10)
			call.outputStore.SetMinBigInt(ord, key, toSet)
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
			call := fromContext(ctx)

			call.validateSetMinFloat64(key)

			call.outputStore.SetMinFloat64(ord, key, value)
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
			call := fromContext(ctx)

			call.validateSetMinFloat64(key)

			toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigDecimal's read of the kv value
			if err != nil {
				call.returnError(fmt.Errorf("parsing bigdecimal: %w", err))
			}

			call.outputStore.SetMinBigDecimal(ord, key, toAdd)
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
			call := fromContext(ctx)

			call.validateSetMaxInt64(key)

			call.outputStore.SetMaxInt64(ord, key, value)
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
			call := fromContext(ctx)

			call.validateSetMaxBigInt(key)

			toSet, _ := new(big.Int).SetString(value, 10)
			call.outputStore.SetMaxBigInt(ord, key, toSet)
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
			call := fromContext(ctx)

			call.validateSetMaxFloat64(key)

			call.outputStore.SetMaxFloat64(ord, key, value)
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
			call := fromContext(ctx)

			call.validateSetMaxFloat64(key)

			toAdd, _, err := big.ParseFloat(value, 10, 100, big.ToNearestEven) // corresponds to SumBigDecimal's read of the kv value
			if err != nil {
				call.returnError(fmt.Errorf("parsing bigdecimal: %w", err))
			}

			call.outputStore.SetMaxBigDecimal(ord, key, toAdd)
		}),
	},

	//	functions["get_at"] = i.getAt
	//	functions["get_first"] = i.getFirst
	//	functions["get_last"] = i.getLast
	//	functions["has_at"] = i.hasAt
	//	functions["has_first"] = i.hasFirst
	//	functions["has_last"] = i.hasLast

	{
		"get_at",
		[]parm{i32, i64, i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			ord := stack[1]
			key := readStringFromStack(mod, stack[2:])
			outputPtr := uint32(stack[4])
			call := fromContext(ctx)

			if int(storeIndex+1) > len(call.inputStores) {
				call.returnError(fmt.Errorf("'get_at' failed: invalid store index %d, %d stores declared", storeIndex, len(call.inputStores)))
			}

			readStore := call.inputStores[storeIndex]
			value, found := readStore.GetAt(ord, key)
			if !found {
				stack[0] = 0
			} else {
				if err := writeOutputToHeap(ctx, mod, outputPtr, value, call.moduleName); err != nil {
					call.returnError(fmt.Errorf("writing output to heap: %w", err))
				}
				stack[0] = 1
			}
			call.traceStateReads("get_at", storeIndex, found, key)
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
			call := fromContext(ctx)

			if int(storeIndex+1) > len(call.inputStores) {
				call.returnError(fmt.Errorf("'has_at' failed: invalid store index %d, %d stores declared", storeIndex, len(call.inputStores)))
			}

			readStore := call.inputStores[storeIndex]
			found := readStore.HasAt(ord, key)
			if !found {
				stack[0] = 0
			} else {
				stack[0] = 1
			}

			call.traceStateReads("has_at", storeIndex, found, key)
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
			call := fromContext(ctx)

			if int(storeIndex+1) > len(call.inputStores) {
				call.returnError(fmt.Errorf("'get_first' failed: invalid store index %d, %d stores declared", storeIndex, len(call.inputStores)))
			}

			readStore := call.inputStores[storeIndex]
			value, found := readStore.GetFirst(key)
			// DRY up here
			if !found {
				stack[0] = 0
			} else {
				if err := writeOutputToHeap(ctx, mod, outputPtr, value, call.moduleName); err != nil {
					call.returnError(fmt.Errorf("writing output to heap: %w", err))
				}
				stack[0] = 1
			}
			call.traceStateReads("get_first", storeIndex, found, key)
		}),
	},
	{
		"has_first",
		[]parm{i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			key := readStringFromStack(mod, stack[1:])
			call := fromContext(ctx)

			if int(storeIndex+1) > len(call.inputStores) {
				call.returnError(fmt.Errorf("'has_first' failed: invalid store index %d, %d stores declared", storeIndex, len(call.inputStores)))
			}

			readStore := call.inputStores[storeIndex]
			found := readStore.HasFirst(key)
			if !found {
				stack[0] = 0
			} else {
				stack[0] = 1
			}
			call.traceStateReads("has_first", storeIndex, found, key)
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
			call := fromContext(ctx)

			if int(storeIndex+1) > len(call.inputStores) {
				call.returnError(fmt.Errorf("'get_last' failed: invalid store index %d, %d stores declared", storeIndex, len(call.inputStores)))
			}

			readStore := call.inputStores[storeIndex]
			value, found := readStore.GetLast(key)
			// DRY up here
			if !found {
				stack[0] = 0
			} else {
				if err := writeOutputToHeap(ctx, mod, outputPtr, value, call.moduleName); err != nil {
					call.returnError(fmt.Errorf("writing output to heap: %w", err))
				}
				stack[0] = 1
			}
			call.traceStateReads("get_last", storeIndex, found, key)
		}),
	},
	{
		"has_last",
		[]parm{i32, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			key := readStringFromStack(mod, stack[1:])
			call := fromContext(ctx)

			if int(storeIndex+1) > len(call.inputStores) {
				call.returnError(fmt.Errorf("'has_last' failed: invalid store index %d, %d stores declared", storeIndex, len(call.inputStores)))
			}

			readStore := call.inputStores[storeIndex]
			found := readStore.HasLast(key)
			if !found {
				stack[0] = 0
			} else {
				stack[0] = 1
			}
			call.traceStateReads("has_last", storeIndex, found, key)
		}),
	},
}
