package execout

import (
	"errors"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var NotFound = errors.New("inputs module value not found")

type CacheEngine interface {
	NewExecOutput(blockType string, block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (ExecutionOutput, error)
	Init(modules *manifest.ModuleHashes) error

	EndOfStream(blockNum uint64) error
	HandleFinal(clock *pbsubstreams.Clock) error
	HandleUndo(clock *pbsubstreams.Clock, moduleName string)
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
