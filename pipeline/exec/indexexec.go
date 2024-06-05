package exec

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/reqctx"

	pbindex "github.com/streamingfast/substreams/pb/sf/substreams/index/v1"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/wasm"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type IndexModuleExecutor struct {
	BaseExecutor
}

func NewIndexModuleExecutor(baseExecutor *BaseExecutor) *IndexModuleExecutor {
	return &IndexModuleExecutor{BaseExecutor: *baseExecutor}
}

func (i *IndexModuleExecutor) Name() string   { return i.moduleName }
func (i *IndexModuleExecutor) String() string { return i.Name() }

func (i *IndexModuleExecutor) applyCachedOutput([]byte) error {
	return nil
}

func (i *IndexModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
	_, span := reqctx.WithModuleExecutionSpan(ctx, "exec_index")
	defer span.EndWithErr(&err)

	var call *wasm.Call
	if call, err = i.wasmCall(reader); err != nil {
		return nil, nil, nil, fmt.Errorf("maps wasm call: %w", err)
	}

	if call != nil {
		out = call.Output()
	}

	modOut, err := i.toModuleOutput(out)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("converting back to module output: %w", err)
	}

	return out, out, modOut, nil
}

func (i *IndexModuleExecutor) toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error) {
	var indexKeys pbindex.Keys
	err := proto.Unmarshal(data, &indexKeys)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling index keys: %w", err)
	}

	return &pbssinternal.ModuleOutput{
		Data: &pbssinternal.ModuleOutput_MapOutput{
			MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/sf.substreams.index.v1.Keys", Value: data},
		},
	}, nil
}

func (i *IndexModuleExecutor) HasValidOutput() bool {
	return true
}
func (i *IndexModuleExecutor) HasOutputForFiles() bool {
	return true
}
