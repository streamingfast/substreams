package exec

import (
	"context"
	"fmt"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"google.golang.org/protobuf/proto"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
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

func (e *StoreModuleExecutor) applyCachedOutput(value []byte) error {
	return e.outputStore.ApplyOps(value)
}

func (e *StoreModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
	_, span := reqctx.WithModuleExecutionSpan(ctx, "exec_store")
	defer span.EndWithErr(&err)

	if _, err := e.wasmCall(reader); err != nil {
		return nil, nil, nil, fmt.Errorf("store wasm call: %w", err)
	}

	return e.wrapDeltasAndOps()
}

func (e *StoreModuleExecutor) HasValidOutput() bool {
	_, ok := e.outputStore.(*store.FullKV)
	return ok
}
func (e *StoreModuleExecutor) HasOutputForFiles() bool {
	return true
}

func (e *StoreModuleExecutor) wrapDeltasAndOps() ([]byte, []byte, *pbssinternal.ModuleOutput, error) {
	if err := e.outputStore.Flush(); err != nil {
		return nil, nil, nil, err
	}

	deltas := &pbsubstreams.StoreDeltas{
		StoreDeltas: e.outputStore.GetDeltas(),
	}

	data, err := proto.Marshal(deltas)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("caching: marshalling delta: %w", err)
	}
	dataForFiles := e.outputStore.ReadOps()

	moduleOutput := &pbssinternal.ModuleOutput{
		Data: &pbssinternal.ModuleOutput_StoreDeltas{
			StoreDeltas: deltas,
		},
	}
	return data, dataForFiles, moduleOutput, nil
}

// toModuleOutput returns nil,nil on partialKV, because we never use the outputs of a partial store directly
func (e *StoreModuleExecutor) toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error) {
	if fullkvs, ok := e.outputStore.(*store.FullKV); ok {
		deltas := fullkvs.GetDeltas()

		return &pbssinternal.ModuleOutput{
			Data: &pbssinternal.ModuleOutput_StoreDeltas{
				StoreDeltas: &pbsubstreams.StoreDeltas{
					StoreDeltas: deltas,
				},
			},
		}, nil
	}
	return nil, nil
}
