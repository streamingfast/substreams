package exec

import (
	"context"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
)

type ModuleExecutor interface {
	// Name returns the name of the module as defined in the manifest.
	Name() string
	String() string
	Close(ctx context.Context) error
	run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error)
	applyCachedOutput(value []byte) error
	toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error)
	HasValidOutput() bool
	HasOutputForFiles() bool

	BlockIndex() *index.BlockIndex
	RunsOnBlock(blockNum uint64) bool

	lastExecutionLogs() (logs []string, truncated bool)
	lastExecutionStack() []string
}
