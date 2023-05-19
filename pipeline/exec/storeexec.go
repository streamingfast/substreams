package exec

import (
	"context"
	"fmt"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"google.golang.org/protobuf/proto"
)

type StoreModuleExecutor struct {
	BaseExecutor
	outputStore store.DeltaAccessor
}

var _ ModuleExecutor = (*StoreModuleExecutor)(nil)

func NewStoreModuleExecutor(baseExecutor *BaseExecutor, outputStore store.DeltaAccessor) *StoreModuleExecutor {
	return &StoreModuleExecutor{BaseExecutor: *baseExecutor, outputStore: outputStore}
}

func (e *StoreModuleExecutor) Name() string   { return e.moduleName }
func (e *StoreModuleExecutor) String() string { return e.Name() }
func (e *StoreModuleExecutor) ResetWASMCall() { e.wasmModule.CurrentCall = nil }

func (e *StoreModuleExecutor) applyCachedOutput(value []byte) error {
	deltas := &pbssinternal.StoreDeltas{}
	err := proto.Unmarshal(value, deltas)
	if err != nil {
		return fmt.Errorf("unmarshalling output deltas: %w", err)
	}
	e.outputStore.SetDeltas(deltas.StoreDeltas)
	return nil
}

func (e *StoreModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "exec_store")
	defer span.EndWithErr(&err)

	if _, err := e.wasmCall(reader); err != nil {
		return nil, nil, fmt.Errorf("store wasm call: %w", err)
	}

	return e.wrapDeltas()
}

func (e *StoreModuleExecutor) HasValidOutput() bool {
	_, ok := e.outputStore.(*store.FullKV)
	return ok
}

func (e *StoreModuleExecutor) wrapDeltas() ([]byte, *pbssinternal.ModuleOutput, error) {
	deltas := &pbssinternal.StoreDeltas{
		StoreDeltas: e.outputStore.GetDeltas(),
	}

	data, err := proto.Marshal(deltas)
	if err != nil {
		return nil, nil, fmt.Errorf("caching: marshalling delta: %w", err)
	}

	moduleOutput := &pbssinternal.ModuleOutput{
		Data: &pbssinternal.ModuleOutput_StoreDeltas{
			StoreDeltas: deltas,
		},
	}
	return data, moduleOutput, nil
}

func (e *StoreModuleExecutor) toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error) {
	deltas := &pbssinternal.StoreDeltas{}
	err := proto.Unmarshal(data, deltas)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling output deltas: %w", err)
	}

	return &pbssinternal.ModuleOutput{
		Data: &pbssinternal.ModuleOutput_StoreDeltas{
			StoreDeltas: deltas,
		},
	}, nil
}
