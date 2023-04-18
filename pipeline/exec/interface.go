package exec

import (
	"context"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/execout"
)

type ModuleExecutor interface {
	// Name returns the name of the module as defined in the manifest.
	Name() string
	String() string
	ResetWASMCall()
	FreeMem()
	run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutputData *pbssinternal.ModuleOutput, err error)
	applyCachedOutput(value []byte) error
	toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error)
	HasValidOutput() bool

	moduleLogs() (logs []string, truncated bool)
	currentExecutionStack() []string
}
