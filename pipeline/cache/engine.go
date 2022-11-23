package cache

import (
	"context"
	"fmt"

	"github.com/streamingfast/bstream"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"go.uber.org/zap"
)

type Engine struct {
	ctx               context.Context
	blockType         string
	reversibleSegment map[uint64]*execout.ExecOutputBuffer // block num to modules' outputs for that given block
	writableFiles     *execout.ExecOutputWriter            // moduleName => irreversible File
	runtimeConfig     config.RuntimeConfig
	logger            *zap.Logger
}

func NewEngine(runtimeConfig config.RuntimeConfig, execoutConfigs *execout.Configs, blockType string, logger *zap.Logger) (*Engine, error) {
	e := &Engine{
		ctx:               context.Background(),
		runtimeConfig:     runtimeConfig,
		reversibleSegment: map[uint64]*execout.ExecOutputBuffer{},
		writableFiles:     &ExecOutputWriter{files: map[string]*execout.File{}},
		logger:            logger,
		blockType:         blockType,
		// caches was: block ID => *execout.File
	}
	return e, nil
}

func (e *Engine) NewExecOutput(block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (execout.ExecutionOutput, error) {
	execOutBuf, err := execout.NewExecOutputBuffer(e.blockType, block, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}

	e.reversibleSegment[clock.Number] = execOutBuf

	return execOutBuf, nil
	//return &cursoredCache{
	//	ExecOutputBuffer: execOutBuf,
	//	engine:           e,
	//	cursor:           cursor.ToOpaque(),
	//}, nil
}

func (e *Engine) HandleUndo(clock *pbsubstreams.Clock) {
	delete(e.reversibleSegment, clock.Number)
}

func (e *Engine) HandleFinal(clock *pbsubstreams.Clock) error {
	// take the e.reversibleSegment[clock.Number]
	// push it into the writableFiles
	// delete(e.reversibleSegment[clock.Number]
	execOutBuf := e.reversibleSegment[clock.Number]

	for _, cache := range e.caches {
		if !cache.IsOutOfRange(clock.Number) {
			continue
		}

		// FIXME(abourget): here we have no guarantee that we only have written
		//  fINALIZED blocks in the cache.  IN fact, we could very well have received
		//  New blocks in the live.
		//  Is that a problem? If we have written blocks in the cache that are not
		//  in the range we're about to write, they'll have been written in the future
		//  (for a cache of 0-100, we might have written 105 and 106).
		//  Will we easily find those back??
		//  Perhaps we should only write caches for finalized blocks, and purge
		//  those block IDs that have not been marked as FINAL from storage.
		//  In file.go:188 we save the `.kv` indistinctly.
		if err := e.flushWritableFiles(cache); err != nil {
			return fmt.Errorf("flushing output cache %s: %w", cache.ModuleName, err)
		}
	}

	// potentially flush the writableFiles if we're there
	return nil
}

func (e *Engine) HandleStalled(clock *pbsubstreams.Clock) error {
	delete(e.reversibleSegment, clock.Id)
	return nil
}

func (e *Engine) EndOfStream(isSubrequest bool, outputModules map[string]bool) error {
	for _, cache := range e.caches {
		if isSubrequest && outputModules[cache.ModuleName] {
			continue
		}
		if err := e.flushWritableFiles(cache); err != nil {
			return fmt.Errorf("flushing output cache %s: %w", cache.ModuleName, err)
		}
	}
	return nil
}

func (e *Engine) flushWritableFiles(cache *execout.File) error {

	err := cache.Save(e.ctx)
	if err != nil {
		return fmt.Errorf("saving cache ouputs: %w", err)
	}

	if _, err := cache.LoadAtEndBlockBoundary(e.ctx); err != nil {
		return fmt.Errorf("loading cache: %w", err)
	}
	return nil
}

func (e *Engine) get(moduleName string, clock *pbsubstreams.Clock) ([]byte, bool, error) {
	cache, found := e.reversibleSegment[clock.Number]
	if !found {
		return nil, false, fmt.Errorf("cache %q not found at block %d", moduleName, clock.Number)
	}

	return cache.Get(moduleName)
	//
	//if !cache.IsInitialized() {
	//	if _, err := cache.LoadAtBlock(e.ctx, clock.Number); err != nil {
	//		return nil, false, fmt.Errorf("unable to load cache %q at block %d: %w", moduleName, clock.Number, err)
	//	}
	//}
	//
	//data, found := cache.Get(clock)
	//return data, found, nil
}

//
//func (e *Engine) set(moduleName string, data []byte, clock *pbsubstreams.Clock, cursor string) error {
//	_ = e.reversibleSegment[clock.Id].Set(moduleName, data)
//	cache, found := e.caches[moduleName]
//	if !found {
//		return fmt.Errorf("cache %q not found", moduleName)
//	}
//
//	return cache.SetItem(clock, cursor, data)
//}

func (e *Engine) Close() error {
	for _, cache := range e.caches {
		cache.Close()
	}
	return nil
}
