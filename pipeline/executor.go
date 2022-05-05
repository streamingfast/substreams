package pipeline

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/state"
	"github.com/streamingfast/substreams/wasm"
)

type ModuleExecutor interface {
	run(vals map[string][]byte, clock *pbsubstreams.Clock, block *bstream.Block) error
	appendOutput(moduleOutputs []*pbsubstreams.ModuleOutput) []*pbsubstreams.ModuleOutput
}

type BaseExecutor struct {
	moduleName string
	wasmModule *wasm.Module
	wasmInputs []*wasm.Input
	cache      *outputCache
	isOutput   bool // whether output is enabled for this module
	entrypoint string
}

type MapperModuleExecutor struct {
	*BaseExecutor
	outputType   string
	mapperOutput []byte
}

type StoreModuleExecutor struct {
	*BaseExecutor
	outputStore *state.Builder
}

func (e *MapperModuleExecutor) run(vals map[string][]byte, clock *pbsubstreams.Clock, block *bstream.Block) error {
	output, found, err := e.cache.get(block)
	if err != nil {
		zlog.Warn("failed to get output from cache", zap.Error(err))
	}

	if found {
		e.mapperOutput = output
		return nil
	}

	if err = e.wasmMapCall(vals, clock); err != nil {
		return err
	}

	if err = e.cache.set(block, e.mapperOutput); err != nil {
		return fmt.Errorf("setting mapper output to cache at block %d: %w", block.Num(), err)
	}

	return nil
}

func (e *StoreModuleExecutor) run(vals map[string][]byte, clock *pbsubstreams.Clock, block *bstream.Block) error {
	output, found, err := e.cache.get(block)
	if err != nil {
		zlog.Warn("failed to get output from cache", zap.Error(err))
	}

	if found {
		deltas := &pbsubstreams.StoreDeltas{}
		err := proto.Unmarshal(output, deltas)
		if err != nil {
			return fmt.Errorf("unmarshalling output deltas: %w", err)
		}
		e.outputStore.Deltas = deltas.Deltas //todo: unmarshall cached data as delta
		for _, delta := range deltas.Deltas {
			e.outputStore.ApplyDelta(delta)
		}
		return nil
	}

	if err = e.wasmStoreCall(vals, clock); err != nil {
		return err
	}

	deltas := &pbsubstreams.StoreDeltas{
		Deltas: e.outputStore.Deltas,
	}
	data, err := proto.Marshal(deltas)
	if err != nil {
		return fmt.Errorf("caching: marshalling delta: %w", err)
	}

	if err = e.cache.set(block, data); err != nil {
		return fmt.Errorf("setting delta to cache at block %d: %w", block.Num(), err)
	}

	return nil
}

func (e *MapperModuleExecutor) wasmMapCall(vals map[string][]byte, clock *pbsubstreams.Clock) (err error) {
	var vm *wasm.Instance
	if vm, err = e.wasmCall(vals, clock); err != nil {
		return err
	}

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

func (e *StoreModuleExecutor) wasmStoreCall(vals map[string][]byte, clock *pbsubstreams.Clock) (err error) {
	if _, err := e.wasmCall(vals, clock); err != nil {
		return err
	}

	return nil
}

func (e *BaseExecutor) wasmCall(vals map[string][]byte, clock *pbsubstreams.Clock) (instance *wasm.Instance, err error) {
	hasInput := false
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
		instance, err = e.wasmModule.NewInstance(clock, e.entrypoint, e.wasmInputs)
		if err != nil {
			return nil, fmt.Errorf("new wasm instance: %w", err)
		}
		if err = instance.Execute(); err != nil {
			return nil, fmt.Errorf("module %q: wasm execution failed: %w", e.moduleName, err)
		}
	}
	return
}

func (e *StoreModuleExecutor) appendOutput(moduleOutputs []*pbsubstreams.ModuleOutput) []*pbsubstreams.ModuleOutput {
	if !e.isOutput {
		return moduleOutputs
	}

	var logs []string
	if e.wasmModule.CurrentInstance != nil {
		logs = e.wasmModule.CurrentInstance.Logs
	}

	if len(e.outputStore.Deltas) != 0 || len(logs) != 0 {
		zlog.Debug("append to output, store")
		moduleOutputs = append(moduleOutputs, &pbsubstreams.ModuleOutput{
			Name: e.moduleName,
			Data: &pbsubstreams.ModuleOutput_StoreDeltas{
				StoreDeltas: &pbsubstreams.StoreDeltas{Deltas: e.outputStore.Deltas},
			},
			Logs: logs,
		})
	}

	return moduleOutputs
}

func (e *MapperModuleExecutor) appendOutput(moduleOutputs []*pbsubstreams.ModuleOutput) []*pbsubstreams.ModuleOutput {
	if !e.isOutput {
		return moduleOutputs
	}

	var logs []string
	if e.wasmModule.CurrentInstance != nil {
		logs = e.wasmModule.CurrentInstance.Logs
	}

	if e.mapperOutput != nil || len(logs) != 0 {
		zlog.Debug("append to output, map")
		moduleOutputs = append(moduleOutputs, &pbsubstreams.ModuleOutput{
			Name: e.moduleName,
			Data: &pbsubstreams.ModuleOutput_MapOutput{
				MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + e.outputType, Value: e.mapperOutput},
			},
			Logs: logs,
		})
	}

	return moduleOutputs
}
