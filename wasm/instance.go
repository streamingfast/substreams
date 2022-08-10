package wasm

import (
	"encoding/binary"
	"fmt"

	"github.com/bytecodealliance/wasmtime-go"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
)

type Instance struct {
	inputStores  []state.Reader
	outputStore  *state.Store
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy

	valueType string

	clock *pbsubstreams.Clock

	args         []interface{} // to the `entrypoint` function
	returnValue  []byte
	panicError   *PanicError
	functionName string
	moduleName   string

	Logs          []string
	LogsByteCount uint64
	Module        *Module
	entrypoint    *wasmtime.Func
}

func (i *Instance) Execute() (err error) {
	if _, err = i.entrypoint.Call(i.Module.wasmStore, i.args...); err != nil {
		if i.panicError != nil {
			return i.panicError
		}
		return fmt.Errorf("executing entrypoint %q: %w", i.functionName, err)
	}
	return nil
}

func (i *Instance) ExecuteWithArgs(args ...interface{}) (err error) {
	if _, err = i.entrypoint.Call(i.Module.wasmStore, args...); err != nil {
		if i.panicError != nil {
			return i.panicError
		}
		return fmt.Errorf("executing with args entrypoint %q: %w", i.functionName, err)
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

func (i *Instance) SetOutputStore(store *state.Store) {
	i.outputStore = store
}

const maxLogByteCount = 128 * 1024 // 128 KiB

func (i *Instance) ReachedLogsMaxByteCount() bool {
	return i.LogsByteCount >= maxLogByteCount
}
