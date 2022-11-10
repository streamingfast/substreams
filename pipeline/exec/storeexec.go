package exec

import (
	"context"
	"fmt"

	store2 "github.com/streamingfast/substreams/storage/store"

	"github.com/streamingfast/substreams/storage/execout"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"google.golang.org/protobuf/proto"
)

type StoreModuleExecutor struct {
	BaseExecutor
	outputStore store2.DeltaAccessor
}

var _ ModuleExecutor = (*StoreModuleExecutor)(nil)

func NewStoreModuleExecutor(baseExecutor *BaseExecutor, outputStore store2.DeltaAccessor) *StoreModuleExecutor {
	return &StoreModuleExecutor{BaseExecutor: *baseExecutor, outputStore: outputStore}
}

func (e *StoreModuleExecutor) Name() string { return e.moduleName }

func (e *StoreModuleExecutor) String() string { return e.Name() }

func (e *StoreModuleExecutor) ResetWASMInstance() { e.wasmModule.CurrentInstance = nil }

func (e *StoreModuleExecutor) applyCachedOutput(value []byte) error {
	deltas := &pbsubstreams.StoreDeltas{}
	err := proto.Unmarshal(value, deltas)
	if err != nil {
		return fmt.Errorf("unmarshalling output deltas: %w", err)
	}
	e.outputStore.SetDeltas(deltas.Deltas)
	return nil
}

func (e *StoreModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutput pbsubstreams.ModuleOutputData, err error) {
	ctx, span := reqctx.WithSpan(ctx, "exec_store")
	defer span.EndWithErr(&err)

	if _, err := e.wasmCall(reader); err != nil {
		return nil, nil, fmt.Errorf("store wasm call: %w", err)
	}

	if e.holdsPartialStore() {
		return nil, nil, nil
	}

	return e.wrapDeltas()
}

func (e *StoreModuleExecutor) holdsPartialStore() bool {
	_, ok := e.outputStore.(*store2.PartialKV)
	return ok
}

func (e *StoreModuleExecutor) wrapDeltas() (out []byte, moduleOutput pbsubstreams.ModuleOutputData, err error) {
	deltas := &pbsubstreams.StoreDeltas{
		Deltas: e.outputStore.GetDeltas(),
	}

	data, err := proto.Marshal(deltas)
	if err != nil {
		return nil, nil, fmt.Errorf("caching: marshalling delta: %w", err)
	}

	moduleOutput = &pbsubstreams.ModuleOutput_DebugStoreDeltas{
		DebugStoreDeltas: deltas,
	}
	return data, moduleOutput, nil
}

func (e *StoreModuleExecutor) toModuleOutput(data []byte) (*pbsubstreams.ModuleOutput, error) {
	deltas := &pbsubstreams.StoreDeltas{}
	err := proto.Unmarshal(data, deltas)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling output deltas: %w", err)
	}
	return toModuleOutput(e, &pbsubstreams.ModuleOutput_DebugStoreDeltas{
		DebugStoreDeltas: deltas,
	}), nil
}
