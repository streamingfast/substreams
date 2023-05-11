package exec

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/anypb"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/wasm"
)

type MapperModuleExecutor struct {
	BaseExecutor
	outputType string
}

var _ ModuleExecutor = (*MapperModuleExecutor)(nil)

func NewMapperModuleExecutor(baseExecutor *BaseExecutor, outputType string) *MapperModuleExecutor {
	return &MapperModuleExecutor{BaseExecutor: *baseExecutor, outputType: outputType}
}

// Name implements ModuleExecutor
func (e *MapperModuleExecutor) Name() string { return e.moduleName }

func (e *MapperModuleExecutor) String() string { return e.Name() }

// todo: this is strange because it has to be done on both the store and the mapper
// and in this case, we don't do anything
func (e *MapperModuleExecutor) applyCachedOutput([]byte) error { return nil }

func (e *MapperModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "exec_map")
	defer span.EndWithErr(&err)

	var call *wasm.Call
	if call, err = e.wasmCall(reader); err != nil {
		return nil, nil, fmt.Errorf("maps wasm call: %w", err)
	}

	if call != nil {
		out = call.Output()
	}

	modOut, err := e.toModuleOutput(out)
	if err != nil {
		return nil, nil, fmt.Errorf("converting back to module output: %w", err)
	}

	return out, modOut, nil
}

func (e *MapperModuleExecutor) toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error) {
	return &pbssinternal.ModuleOutput{
		Data: &pbssinternal.ModuleOutput_MapOutput{
			MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + e.outputType, Value: data},
		},
	}, nil
}

func (e *MapperModuleExecutor) HasValidOutput() bool {
	return true
}
