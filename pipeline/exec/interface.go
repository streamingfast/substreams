package exec

import (
	"context"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
)

type ModuleExecutor interface {
	// Name returns the name of the module as defined in the manifest.
	Name() string
	String() string
	ResetWASMInstance()
	FreeMem()
	run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutputData pbsubstreams.ModuleOutputData, err error)
	applyCachedOutput(value []byte) error
	toModuleOutput(data []byte) (*pbsubstreams.ModuleOutput, error)
	outputCacheable() bool

	moduleLogs() (logs []string, truncated bool)
	currentExecutionStack() []string
}
