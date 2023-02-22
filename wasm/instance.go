package wasm

import (
	"encoding/binary"
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/storage/store"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Instance struct {
	inputStores  []store.Reader
	outputStore  store.Store
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy

	valueType string

	clock *pbsubstreams.Clock

	args        []interface{} // to the `entrypoint` function
	returnValue []byte
	panicError  *PanicError

	Logs           []string
	LogsByteCount  uint64
	ExecutionStack []string
	Module         *Module
	entrypoint     *wasmtime.Func
}

func (i *Instance) Execute() (err error) {
	if _, err = i.entrypoint.Call(i.Module.wasmStore, i.args...); err != nil {
		if i.panicError != nil {
			return i.panicError
		}
		return fmt.Errorf("executing module %q: %w", i.Module.name, err)
	}
	return nil
}

func (i *Instance) ExecuteWithArgs(args ...interface{}) (err error) {
	if _, err = i.entrypoint.Call(i.Module.wasmStore, args...); err != nil {
		if i.panicError != nil {
			return i.panicError
		}
		return fmt.Errorf("executing module with args %q: %w", i.Module.name, err)
	}
	return nil
}

func (i *Instance) WriteOutputToHeap(outputPtr int32, value []byte, from string) error {
	valuePtr, err := i.Module.Heap.WriteAndTrack(value, false, from+":WriteOutputToHeap1")
	if err != nil {
		return fmt.Errorf("writting value to heap: %w", err)
	}
	returnValue := make([]byte, 8)
	binary.LittleEndian.PutUint32(returnValue[0:4], uint32(valuePtr))
	binary.LittleEndian.PutUint32(returnValue[4:], uint32(len(value)))

	_, err = i.Module.Heap.WriteAtPtr(returnValue, outputPtr, from+":WriteOutputToHeap2")
	if err != nil {
		return fmt.Errorf("writing response at valuePtr %d: %w", valuePtr, err)
	}

	return nil
}

func (i *Instance) Err() error {
	return i.panicError
}

func (i *Instance) Output() []byte {
	return i.returnValue
}

func (i *Instance) SetOutputStore(store store.Store) {
	i.outputStore = store
}

const maxLogByteCount = 128 * 1024 // 128 KiB

func (i *Instance) ReachedLogsMaxByteCount() bool {
	return i.LogsByteCount >= maxLogByteCount
}

func (i *Instance) PushExecutionStack(event string) {
	i.ExecutionStack = append(i.ExecutionStack, event)
}

func (i *Instance) Cleanup() error {
	err := i.Module.Heap.Clear()
	if err != nil {
		return fmt.Errorf("clearing heap: %w", err)
	}
	i.Module.wasmStore.GC()
	return err
}

func (i *Instance) IsValidSetStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_SET
}

func (i *Instance) IsValidSetIfNotExists() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS
}

func (i *Instance) IsValidAppendStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND
}

func (i *Instance) IsValidAddBigIntStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && i.valueType == manifest.OutputValueTypeBigInt
}

func (i *Instance) IsValidAddBigDecimalStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD &&
		(i.valueType == manifest.OutputValueTypeBigDecimal || i.valueType == manifest.OutputValueTypeBigFloat)
}

func (i *Instance) IsValidAddInt64Store() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && i.valueType == manifest.OutputValueTypeInt64
}

func (i *Instance) IsValidAddFloat64Store() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD && i.valueType == manifest.OutputValueTypeFloat64
}

func (i *Instance) IsValidSetMinInt64Store() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && i.valueType == manifest.OutputValueTypeInt64
}

func (i *Instance) IsValidSetMinBigIntStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && i.valueType == manifest.OutputValueTypeBigInt
}

func (i *Instance) IsValidSetMinFloat64Store() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN && i.valueType == manifest.OutputValueTypeFloat64
}

func (i *Instance) IsValidSetMinBigDecimalStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN &&
		(i.valueType == manifest.OutputValueTypeBigDecimal || i.valueType == manifest.OutputValueTypeBigFloat)
}

func (i *Instance) IsValidSetMaxInt64Store() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		i.valueType == manifest.OutputValueTypeInt64
}

func (i *Instance) IsValidSetMaxBigIntStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		i.valueType == manifest.OutputValueTypeBigInt
}

func (i *Instance) IsValidSetMaxFloat64Store() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		i.valueType == manifest.OutputValueTypeFloat64
}

func (i *Instance) IsValidSetMaxBigDecimalStore() bool {
	return i.updatePolicy == pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX &&
		(i.valueType == manifest.OutputValueTypeBigDecimal || i.valueType == manifest.OutputValueTypeBigFloat)
}
