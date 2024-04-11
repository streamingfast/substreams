package cache

import (
	"context"
	"fmt"

	multierror "github.com/hashicorp/go-multierror"
	pbindex "github.com/streamingfast/substreams/pb/sf/substreams/index/v1"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/streamingfast/substreams/storage/index"
	"google.golang.org/protobuf/proto"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"

	"github.com/streamingfast/substreams/reqctx"

	"github.com/streamingfast/bstream"
	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
)

// Engine manages the reversible segments and keeps track of
// the execution output between each module's.
//
// Upon Finality, it writes it to some output cache files.
type Engine struct {
	// FIXME: Rename to pipeline.Lifecycle ? to hold also the *pbsubstreams.ModuleOutput
	//  so that `ForkHandler` disappears in the end?
	ctx               context.Context
	blockType         string
	reversibleBuffers map[uint64]*execout.Buffer // block num to modules' outputs for that given block
	execOutputWriters map[string]*execout.Writer // moduleName => writer (single file)
	existingExecOuts  map[string]*execout.File
	indexWriters      map[string]*index.Writer

	runtimeConfig config.RuntimeConfig // TODO(abourget): Deprecated: remove this as it's not used
	logger        *zap.Logger
}

func NewEngine(ctx context.Context, runtimeConfig config.RuntimeConfig, execOutWriters map[string]*execout.Writer, blockType string, existingExecOuts map[string]*execout.File, indexWriters map[string]*index.Writer) (*Engine, error) {
	e := &Engine{
		ctx:               ctx,
		runtimeConfig:     runtimeConfig,
		reversibleBuffers: map[uint64]*execout.Buffer{},
		execOutputWriters: execOutWriters,
		logger:            reqctx.Logger(ctx),
		blockType:         blockType,
		indexWriters:      indexWriters,
		existingExecOuts:  existingExecOuts,
	}
	return e, nil
}

func (e *Engine) NewBuffer(optionalBlock *pbbstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (execout.ExecutionOutput, error) {
	out, err := execout.NewBuffer(e.blockType, optionalBlock, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}

	e.reversibleBuffers[clock.Number] = out
	for moduleName, existingExecOut := range e.existingExecOuts {
		if val, ok := existingExecOut.Get(clock); ok {
			out.Set(moduleName, val)
		}
	}

	return out, nil
}

func (e *Engine) HandleUndo(clock *pbsubstreams.Clock) {
	delete(e.reversibleBuffers, clock.Number)
}

func (e *Engine) HandleFinal(clock *pbsubstreams.Clock) error {
	execOutBuf := e.reversibleBuffers[clock.Number]
	if execOutBuf == nil {
		// TODO(abourget): cross check here, do we want to defer the MaybeRotate
		//  at after?
		return nil
	}

	for _, writer := range e.execOutputWriters {
		writer.Write(clock, execOutBuf)
	}

	//TODO: Save all indexes from the execOut of blockIndexes modules

	delete(e.reversibleBuffers, clock.Number)

	return nil
}

func (e *Engine) HandleStalled(clock *pbsubstreams.Clock) error {
	delete(e.reversibleBuffers, clock.Number)
	return nil
}

func (e *Engine) EndOfStream(lastFinalClock *pbsubstreams.Clock) error {
	var errs error
	for _, writer := range e.execOutputWriters {
		if err := writer.Close(context.Background()); err != nil {
			errs = multierror.Append(errs, err)
		}
		currentFile := writer.CurrentFile

		if e.indexWriters != nil {
			if indexWriter, ok := e.indexWriters[currentFile.ModuleName]; ok {
				indexes := make(map[string]*roaring64.Bitmap)
				for _, item := range currentFile.Kv {
					blockIndexOutput := item.Payload
					extractedKeys := &pbindex.Keys{}
					err := proto.Unmarshal(blockIndexOutput, extractedKeys)
					if err != nil {
						return fmt.Errorf("unmarshalling index keys from %w outputs: %w", currentFile.ModuleName, err)
					}

					for _, key := range extractedKeys.Keys {
						if _, ok = indexes[key]; !ok {
							indexes[key] = roaring64.New()
						}
						indexes[key].Add(item.BlockNum)
					}
				}

				indexWriter.Write(indexes)

				//TODO: HANDLE THE ERROR
				err := indexWriter.Close(context.Background())
				if err != nil {
				}
			}
		}
	}

	return errs
}
