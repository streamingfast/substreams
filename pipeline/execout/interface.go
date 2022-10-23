package execout

import (
	"context"
	"errors"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var NotFound = errors.New("inputs module value not found")

type CacheEngine interface {
	NewExecOutput(blockType string, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (ExecutionOutput, error)
	Init(modules *manifest.ModuleHashes) error

	EndOfStream(ctx context.Context, clock *pbsubstreams.Clock) error
	HandleFinal(ctx context.Context, clock *pbsubstreams.Clock) error
	//IRRBlock(ctx context.Context, clock *pbsubstreams.Clock) error
	//NewBlock(blockRef bstream.BlockRef) error
	//NewBlock(blockRef bstream.BlockRef, step bstream.StepType) error
}

type ExecutionOutputGetter interface {
	Clock() *pbsubstreams.Clock
	Get(name string) (value []byte, cached bool, err error)
}

type ExecutionOutputSetter interface {
	Set(name string, value []byte) (err error)
}

// ExecutionOutput Execution output for a given graph at a given block
type ExecutionOutput interface {
	ExecutionOutputGetter
	ExecutionOutputSetter
}
