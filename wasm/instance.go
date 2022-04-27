package wasm

import (
	"encoding/binary"
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/wasmerio/wasmer-go/wasmer"
)

type Instance struct {
	memory       *wasmer.Memory
	heap         *Heap
	store        *wasmer.Store
	inputStores  []state.Reader
	outputStore  *state.Builder
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy

	valueType  string
	entrypoint *wasmer.Function

	clock *pbsubstreams.Clock

	args         []interface{} // to the `entrypoint` function
	returnValue  []byte
	panicError   *PanicError
	functionName string
	vmInstance   *wasmer.Instance
	moduleName   string

	Logs []string
}

func (i *Instance) Heap() *Heap {
	return i.heap
}

func (i *Instance) Store() *wasmer.Store {
	return i.store
}

func (i *Instance) PrintStats() {
	fmt.Printf("Memory size: %d\n", i.memory.DataSize())
}

func (i *Instance) Close() {
	i.vmInstance.Close()
}

func (i *Instance) Execute() (err error) {
	if _, err = i.entrypoint.Call(i.args...); err != nil {
		return fmt.Errorf("executing entrypoint %q: %w", i.functionName, err)
	}
	return nil
}

func (i *Instance) ExecuteWithArgs(args ...interface{}) (err error) {
	if _, err = i.entrypoint.Call(args...); err != nil {
		return fmt.Errorf("executing with args entrypoint %q: %w", i.functionName, err)
	}
	return nil
}

func (i *Instance) WriteOutputToHeap(outputPtr int32, value []byte) error {

	valuePtr, err := i.heap.Write(value)
	if err != nil {
		return fmt.Errorf("writting value to heap: %w", err)
	}
	returnValue := make([]byte, 8)
	binary.LittleEndian.PutUint32(returnValue[0:4], uint32(valuePtr))
	binary.LittleEndian.PutUint32(returnValue[4:], uint32(len(value)))

	_, err = i.heap.WriteAtPtr(returnValue, outputPtr)
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

func (i *Instance) SetOutputStore(store *state.Builder) {
	i.outputStore = store
}
