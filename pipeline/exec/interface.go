package exec

import (
	"context"

	"github.com/streamingfast/substreams/storage/execout"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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
	HasValidOutput() bool

	moduleLogs() (logs []string, truncated bool)
	currentExecutionStack() []string
}
