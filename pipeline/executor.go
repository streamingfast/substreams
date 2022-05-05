package pipeline

import (
	"fmt"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/types/known/anypb"
)

type ModuleExecutor struct {
	moduleName string
	wasmModule *wasm.Module
	wasmInputs []*wasm.Input
	pipeline   *Pipeline
	isStore    bool
	isOutput   bool // whether output is enabled for this module

	outputStore *state.Builder

	mapperOutput []byte
	outputType   string
	entrypoint   string
}

func (e *ModuleExecutor) run() (err error) {
	if e.isStore {
		err = e.wasmStoreCall()
	} else {
		err = e.wasmMapCall()
	}
	if err != nil {
		return err
	}

	return nil
}
func (e *ModuleExecutor) wasmMapCall() (err error) {
	var vm *wasm.Instance
	if vm, err = e.wasmCall(); err != nil {
		return err
	}

	vals := e.pipeline.wasmOutputs
	name := e.moduleName
	if vm != nil {
		out := vm.Output()
		vals[name] = out
		e.mapperOutput = out

	} else {
		// This means wasm execution was skipped because all inputs were empty.
		vals[name] = nil
		e.mapperOutput = nil
	}
	return nil
}

func (e *ModuleExecutor) wasmStoreCall() (err error) {
	if _, err := e.wasmCall(); err != nil {
		return err
	}

	return nil
}

func (e *ModuleExecutor) wasmCall() (instance *wasm.Instance, err error) {
	hasInput := false
	vals := e.pipeline.wasmOutputs
	for _, input := range e.wasmInputs {
		switch input.Type {
		case wasm.InputSource:
			val := vals[input.Name]
			if len(val) != 0 {
				input.StreamData = val
				hasInput = true
			} else {
				input.StreamData = nil
			}
		case wasm.InputStore:
			hasInput = true
		case wasm.OutputStore:

		default:
			panic(fmt.Sprintf("Invalid input type %d", input.Type))
		}
	}

	// This allows us to skip the execution of the VM if there are no inputs.
	// This assumption should either be configurable by the manifest, or clearly documented:
	//  state builders will not be called if their input streams are 0 bytes length (and there'e no
	//  state store in read mode)
	if hasInput {
		instance, err = e.wasmModule.NewInstance(e.pipeline.currentClock, e.entrypoint, e.wasmInputs)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}
		if err = instance.Execute(); err != nil {
			return nil, fmt.Errorf("module %q: wasm execution failed: %w", e.moduleName, err)
		}
	}
	return
}

func (e *ModuleExecutor) appendOutput() {
	if !e.isOutput {
		return
	}

	var logs []string
	if e.wasmModule.CurrentInstance != nil {
		logs = e.wasmModule.CurrentInstance.Logs
	}

	if e.isStore {
		if len(e.outputStore.Deltas) != 0 || len(logs) != 0 {
			zlog.Debug("append to output, store")
			e.pipeline.moduleOutputs = append(e.pipeline.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name: e.moduleName,
				Data: &pbsubstreams.ModuleOutput_StoreDeltas{
					StoreDeltas: &pbsubstreams.StoreDeltas{Deltas: e.outputStore.Deltas},
				},
				Logs: logs,
			})
		}
	} else {
		if e.mapperOutput != nil || len(logs) != 0 {
			zlog.Debug("append to output, map")
			e.pipeline.moduleOutputs = append(e.pipeline.moduleOutputs, &pbsubstreams.ModuleOutput{
				Name: e.moduleName,
				Data: &pbsubstreams.ModuleOutput_MapOutput{
					MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + e.outputType, Value: e.mapperOutput},
				},
				Logs: logs,
			})
		}
	}
}
