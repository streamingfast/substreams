package wasmtime

import (
	"fmt"

	pbstore "github.com/streamingfast/substreams-foundational-store/pb/sf/substreams/foundational-store/v1"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/proto"
)

func (i *instance) set(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadBytes(valPtr, valLength)
	i.CurrentCall.DoSet(uint64(ord), key, value)
}

func (i *instance) setIfNotExists(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadBytes(valPtr, valLength)
	i.CurrentCall.DoSetIfNotExists(uint64(ord), key, value)
}

func (i *instance) append(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadBytes(valPtr, valLength)
	i.CurrentCall.DoAppend(uint64(ord), key, value)
}

func (i *instance) deletePrefix(ord int64, keyPtr, keyLength int32) {
	prefix := i.Heap.ReadString(keyPtr, keyLength)
	i.CurrentCall.DoDeletePrefix(uint64(ord), prefix)
}

func (i *instance) setSumInt64(ord int64, keyPtr, keyLength int32, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetSumInt64(uint64(ord), key, value)
}

func (i *instance) setSumFloat64(ord int64, keyPtr, keyLength int32, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetSumFloat64(uint64(ord), key, value)
}

func (i *instance) setSumBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetSumBigInt(uint64(ord), key, value)
}

func (i *instance) setSumBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetSumBigDecimal(uint64(ord), key, value)
}

func (i *instance) addBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoAddBigInt(uint64(ord), key, value)
}

func (i *instance) addBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoAddBigDecimal(uint64(ord), key, value)
}

func (i *instance) addInt64(ord int64, keyPtr, keyLength int32, value int64) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	i.CurrentCall.DoAddInt64(uint64(ord), key, value)
}

func (i *instance) addFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	i.CurrentCall.DoAddFloat64(uint64(ord), key, value)
}

func (i *instance) setMinInt64(ord int64, keyPtr, keyLength int32, value int64) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	i.CurrentCall.DoSetMinInt64(uint64(ord), key, value)
}

func (i *instance) setMinBigint(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetMinBigInt(uint64(ord), key, value)
}

func (i *instance) setMinFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	i.CurrentCall.DoSetMinFloat64(uint64(ord), key, value)
}

func (i *instance) setMinBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetMinBigDecimal(uint64(ord), key, value)
}

func (i *instance) setMaxInt64(ord int64, keyPtr, keyLength int32, value int64) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	i.CurrentCall.DoSetMaxInt64(uint64(ord), key, value)
}

func (i *instance) setMaxBigInt(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetMaxBigInt(uint64(ord), key, value)
}

func (i *instance) setMaxFloat64(ord int64, keyPtr, keyLength int32, value float64) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	i.CurrentCall.DoSetMaxFloat64(uint64(ord), key, value)
}

func (i *instance) setMaxBigDecimal(ord int64, keyPtr, keyLength, valPtr, valLength int32) {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value := i.Heap.ReadString(valPtr, valLength)
	i.CurrentCall.DoSetMaxBigDecimal(uint64(ord), key, value)
}

func (i *instance) getAt(storeIndex int32, ord int64, keyPtr, keyLength, outputPtr int32) int32 {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value, found := i.CurrentCall.DoGetAt(int(storeIndex), uint64(ord), key)
	return writeToHeapIfFound(i, outputPtr, value, found)
}

func (i *instance) hasAt(storeIndex int32, ord int64, keyPtr, keyLength int32) int32 {
	key := i.Heap.ReadString(keyPtr, keyLength)
	found := i.CurrentCall.DoHasAt(int(storeIndex), uint64(ord), key)
	return returnIfFound(found)
}

func (i *instance) getFirst(storeIndex int32, keyPtr, keyLength, outputPtr int32) int32 {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value, found := i.CurrentCall.DoGetFirst(int(storeIndex), key)
	return writeToHeapIfFound(i, outputPtr, value, found)
}

func (i *instance) hasFirst(storeIndex int32, keyPtr, keyLength int32) int32 {
	key := i.Heap.ReadString(keyPtr, keyLength)
	found := i.CurrentCall.DoHasFirst(int(storeIndex), key)
	return returnIfFound(found)
}

func (i *instance) getLast(storeIndex int32, keyPtr, keyLength, outputPtr int32) int32 {
	key := i.Heap.ReadString(keyPtr, keyLength)
	value, found := i.CurrentCall.DoGetLast(int(storeIndex), key)
	return writeToHeapIfFound(i, outputPtr, value, found)
}

func (i *instance) hasLast(storeIndex int32, keyPtr, keyLength int32) int32 {
	key := i.Heap.ReadString(keyPtr, keyLength)
	found := i.CurrentCall.DoHasLast(int(storeIndex), key)
	return returnIfFound(found)
}

func (i *instance) foundationalStoreGet(storeIndex int32, reqPtr int32, reqLen int32) int64 {
	reqData := i.Heap.ReadBytes(reqPtr, reqLen)

	// Deserialize GetRequest
	var req pbstore.GetRequest
	if err := proto.Unmarshal(reqData, &req); err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("failed to unmarshal GetRequest: %w", err))
		return 0
	}

	// Inject current Clock if found
	blockNumber := req.BlockNumber
	blockHash := req.BlockHash
	if blockNumber == 0 && len(blockHash) == 0 {
		if i.CurrentCall.Clock != nil {
			blockNumber = i.CurrentCall.Clock.Number
			if decoded := wasm.DecodeHashString(i.CurrentCall.Clock.Id); decoded != nil {
				blockHash = decoded
			}
		}
	}

	resp, err := i.CurrentCall.DoFoundationalStoreGet(uint32(storeIndex), blockNumber, blockHash, req.Key)
	if err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("foundational store error: %w", err))
		return 0
	}
	// Serialize response
	respData, err := proto.Marshal(resp)
	if err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("failed to marshal GetResponse: %w", err))
		return 0
	}

	// Write to heap and return packed pointer/length
	respPtr, err := i.Heap.Write(respData, "foundationalStoreGet")
	if err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("writing response to heap: %w", err))
		return 0
	}

	// Pack pointer and length into int64
	return packPtrLen(respPtr, int32(len(respData)))
}

func (i *instance) foundationalStoreGetAll(storeIndex int32, reqPtr int32, reqLen int32) int64 {
	reqData := i.Heap.ReadBytes(reqPtr, reqLen)

	// Deserialize GetAllRequest
	var req pbstore.GetAllRequest
	if err := proto.Unmarshal(reqData, &req); err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("failed to unmarshal GetAllRequest: %w", err))
		return 0
	}

	// Inject current Clock if found
	blockNumber := req.BlockNumber
	blockHash := req.BlockHash
	if blockNumber == 0 && len(blockHash) == 0 {
		if i.CurrentCall.Clock != nil {
			blockNumber = i.CurrentCall.Clock.Number
			if decoded := wasm.DecodeHashString(i.CurrentCall.Clock.Id); decoded != nil {
				blockHash = decoded
			}
		}
	}

	resp, err := i.CurrentCall.DoFoundationalStoreGetAll(uint32(storeIndex), blockNumber, blockHash, req.Keys)
	if err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("foundational store error: %w", err))
		return 0
	}

	// Serialize response
	respData, err := proto.Marshal(resp)
	if err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("failed to marshal GetAllResponse: %w", err))
		return 0
	}

	// Write to heap and return packed pointer/length
	respPtr, err := i.Heap.Write(respData, "foundationalStoreGetAll")
	if err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("writing response to heap: %w", err))
		return 0
	}

	// Pack pointer and length into int64
	return packPtrLen(respPtr, int32(len(respData)))
}

// Helper function to pack pointer and length into int64
func packPtrLen(ptr, length int32) int64 {
	return int64(ptr)<<32 | int64(length)
}

func writeToHeapIfFound(i *instance, outputPtr int32, value []byte, found bool) int32 {
	if !found {
		return 0
	}
	if err := writeOutputToHeap(i, outputPtr, value); err != nil {
		i.CurrentCall.ReturnError(fmt.Errorf("writing output to heap: %w", err))
	}
	return 1
}

func returnIfFound(found bool) int32 {
	if found {
		return 1
	}
	return 0
}
