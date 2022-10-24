package exec

import (
	"context"
	"github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
)

type ModuleExecutor interface {
	// Name returns the name of the module as defined in the manifest.
	Name() string

	// String returns the module executor representation, usually its name directly.
	String() string

	// Reset the wasm instance, avoid propagating logs.
	Reset()

	run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutputData pbsubstreams.ModuleOutputData, err error)
	applyCachedOutput(value []byte) error

	moduleLogs() (logs []string, truncated bool)
	currentExecutionStack() []string
}
