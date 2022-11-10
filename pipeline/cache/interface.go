package cache

import (
	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type CacheEngine interface {
	NewExecOutput(block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (execout.ExecutionOutput, error)
	Init(modules *manifest.ModuleHashes) error

	EndOfStream(isSubrequest bool, outputModules map[string]bool) error
	HandleFinal(clock *pbsubstreams.Clock) error
	HandleUndo(clock *pbsubstreams.Clock, moduleName string)
}
