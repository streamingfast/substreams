package exec

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/storage/execout"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/types/known/anypb"
)

type MapperModuleExecutor struct {
	BaseExecutor
	outputType string
}

var _ ModuleExecutor = (*MapperModuleExecutor)(nil)

func NewMapperModuleExecutor(baseExecutor *BaseExecutor, outputType string) *MapperModuleExecutor {
	return &MapperModuleExecutor{BaseExecutor: *baseExecutor, outputType: outputType}
}

var _ ModuleExecutor = (*StoreModuleExecutor)(nil)

// Name implements ModuleExecutor
func (e *MapperModuleExecutor) Name() string { return e.moduleName }

func (e *MapperModuleExecutor) String() string { return e.Name() }

func (e *MapperModuleExecutor) ResetWASMInstance() { e.wasmModule.CurrentInstance = nil }

// todo: this is strange because it has to be done on both the store and the mapper
// and in this case, we don't do anything
func (e *MapperModuleExecutor) applyCachedOutput([]byte) error { return nil }

func (e *MapperModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutput pbsubstreams.ModuleOutputData, err error) {
	ctx, span := reqctx.WithSpan(ctx, "exec_map")
	defer span.EndWithErr(&err)

	var instance *wasm.Instance
	if instance, err = e.wasmCall(reader); err != nil {
		return nil, nil, fmt.Errorf("maps wasm call: %w", err)
	}

	if instance != nil {
		out = instance.Output()
	}

	if out != nil {
		moduleOutput = &pbsubstreams.ModuleOutput_MapOutput{
			MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + e.outputType, Value: out},
		}
	}

	return out, moduleOutput, nil
}

func (e *MapperModuleExecutor) toModuleOutput(data []byte) (*pbsubstreams.ModuleOutput, error) {
	return toModuleOutput(e, &pbsubstreams.ModuleOutput_MapOutput{
		MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + e.outputType, Value: data},
	}), nil
}

func (e *MapperModuleExecutor) HasValidOutput() bool {
	return true
}
