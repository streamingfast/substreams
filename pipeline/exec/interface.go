package exec

import (
	"context"

	"github.com/RoaringBitmap/roaring/roaring64"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/sqe"
	"github.com/streamingfast/substreams/storage/execout"
)

type ModuleExecutor interface {
	// Name returns the name of the module as defined in the manifest.
	Name() string
	String() string
	Close(ctx context.Context) error
	run(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, moduleOutputData *pbssinternal.ModuleOutput, err error)
	applyCachedOutput(value []byte) error
	toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error)
	HasValidOutput() bool

	BlockIndexExcludesAllBlocks() bool
	BlockIndexExpression() sqe.Expression
	BlockIndexModule() string
	BlockIndices() *roaring64.Bitmap

	lastExecutionLogs() (logs []string, truncated bool)
	lastExecutionStack() []string
}
