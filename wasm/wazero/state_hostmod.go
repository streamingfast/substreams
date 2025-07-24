package wazero

import (
	"context"
	"fmt"

	pbstore "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/v1"
	"github.com/streamingfast/substreams/wasm"
	"github.com/tetratelabs/wazero/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type parm = api.ValueType

var i32 = api.ValueTypeI32
var i64 = api.ValueTypeI64
var f64 = api.ValueTypeF64

type funcs struct {
	name  string
	input []parm
	//inputNames  []string
	output []parm
	f      api.GoModuleFunction
}

var StateFuncs = []funcs{
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
		"set_sum_bigint",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetSumBigInt(ord, key, value)
		}),
	},
	{
		"set_sum_bigdecimal",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetSumBigDecimal(ord, key, value)
		}),
	},
	{
		"set_sum_int64",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetSumInt64(ord, key, value)
		}),
	},
	{
		"set_sum_float64",
		[]parm{i64, i32, i32, i32, i32},
		[]parm{},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			ord := stack[0]
			key := readStringFromStack(mod, stack[1:])
			value := readStringFromStack(mod, stack[3:])
			call := wasm.FromContext(ctx)

			call.DoSetSumFloat64(ord, key, value)
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
			inst := instanceFromContext(ctx)

			value, found := call.DoGetAt(int(storeIndex), ord, key)
			setStackAndOutput(ctx, stack, call, found, inst, outputPtr, value)
		}),
	},
	{
		"has_at",
		[]parm{i32, i64, i32, i32},
		[]parm{i32},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			ord := stack[1]
			key := readStringFromStack(mod, stack[2:])
			call := wasm.FromContext(ctx)

			found := call.DoHasAt(int(storeIndex), ord, key)
			setStack0Bool(stack, found)
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
			inst := instanceFromContext(ctx)

			value, found := call.DoGetFirst(int(storeIndex), key)
			setStackAndOutput(ctx, stack, call, found, inst, outputPtr, value)
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
			setStack0Bool(stack, found)
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
			inst := instanceFromContext(ctx)

			value, found := call.DoGetLast(int(storeIndex), key)
			setStackAndOutput(ctx, stack, call, found, inst, outputPtr, value)
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
			setStack0Bool(stack, found)
		}),
	},
	{
		"foundational_store_get",
		[]parm{i32, i32, i32},
		[]parm{i64},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			reqData := readBytesFromStack(mod, stack[1:])
			call := wasm.FromContext(ctx)
			inst := instanceFromContext(ctx)

			// Deserialize
			var req pbstore.GetRequest
			if err := proto.Unmarshal(reqData, &req); err != nil {
				call.ReturnError(fmt.Errorf("failed to unmarshal GetRequest: %w", err))
				stack[0] = 0
				return
			}

			resp, err := call.DoFStoreGet(storeIndex, req.BlockNumber, req.Key)
			if err != nil {
				call.ReturnError(fmt.Errorf("foundational store error: %w", err))
				stack[0] = 0
				return
			}

			// Serialize response
			respData, err := proto.Marshal(resp)
			if err != nil {
				call.ReturnError(fmt.Errorf("failed to marshal GetResponse: %w", err))
				stack[0] = 0
				return
			}

			respPtr, err := writeToHeap(ctx, inst, true, respData)
			if err != nil {
				call.ReturnError(fmt.Errorf("writing response to heap: %w", err))
				stack[0] = 0
				return
			}

			stack[0] = packPtrLen(respPtr, uint32(len(respData)))
		}),
	},
	{
		"foundational_store_get_all",
		[]parm{i32, i32, i32},
		[]parm{i64},
		api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			storeIndex := uint32(stack[0])
			reqData := readBytesFromStack(mod, stack[1:])
			call := wasm.FromContext(ctx)
			inst := instanceFromContext(ctx)

			// Deserialize
			var req pbstore.GetAllRequest
			if err := proto.Unmarshal(reqData, &req); err != nil {
				call.ReturnError(fmt.Errorf("failed to unmarshal GetAllRequest: %w", err))
				stack[0] = 0
				return
			}

			vals := call.DoFoundationalStoreGetAll(storeIndex, req.BlockNumber, req.Keys)

			resp := &pbstore.GetAllResponse{}

			for _, key := range req.Keys {
				entry := &pbstore.ResponseEntry{
					Key: key,
				}
				if val, exists := vals[string(key)]; exists {
					entry.Response = &pbstore.GetResponse{
						Response: pbstore.ResponseCode_FOUND,
						Value:    &anypb.Any{Value: val},
					}
				} else {
					entry.Response = &pbstore.GetResponse{Response: pbstore.ResponseCode_NOT_FOUND}
				}
				resp.Entries = append(resp.Entries, entry)
			}

			// Serialize response
			respData, err := proto.Marshal(resp)
			if err != nil {
				call.ReturnError(fmt.Errorf("failed to marshal GetAllResponse: %w", err))
				stack[0] = 0
				return
			}

			respPtr, err := writeToHeap(ctx, inst, true, respData)
			if err != nil {
				call.ReturnError(fmt.Errorf("writing response to heap: %w", err))
				stack[0] = 0
				return
			}

			stack[0] = packPtrLen(respPtr, uint32(len(respData)))
		}),
	},
}

func setStackAndOutput(ctx context.Context, stack []uint64, call *wasm.Call, found bool, inst *Instance, outputPtr uint32, value []byte) {
	if !found {
		stack[0] = 0
	} else {
		if err := writeOutputToHeap(ctx, inst, outputPtr, value); err != nil {
			call.ReturnError(fmt.Errorf("writing output to heap: %w", err))
		}
		stack[0] = 1
	}
}

func setStack0Bool(stack []uint64, value bool) {
	if value {
		stack[0] = 1
	} else {
		stack[0] = 0
	}
}

// Helper function to pack pointer and length into uint64
func packPtrLen(ptr, length uint32) uint64 {
	return uint64(ptr)<<32 | uint64(length)
}
