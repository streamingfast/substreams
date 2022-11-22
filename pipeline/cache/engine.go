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
	ctx           context.Context
	blockType     string
	caches        map[string]*execout.File
	runtimeConfig config.RuntimeConfig
	logger        *zap.Logger
}

func NewEngine(runtimeConfig config.RuntimeConfig, execoutConfigs *execout.Configs, blockType string, logger *zap.Logger) (*Engine, error) {
	e := &Engine{
		ctx:           context.Background(),
		runtimeConfig: runtimeConfig,
		caches:        execoutConfigs.NewFiles(logger),
		logger:        logger,
		blockType:     blockType,
	}
	return e, nil
}

func (e *Engine) EndOfStream(isSubrequest bool, outputModules map[string]bool) error {
	for _, cache := range e.caches {
		if isSubrequest && outputModules[cache.ModuleName] {
			continue
		}
		if err := e.flushCache(cache); err != nil {
			return fmt.Errorf("flushing output cache %s: %w", cache.ModuleName, err)
		}
	}
	return nil
}

func (e *Engine) HandleFinal(clock *pbsubstreams.Clock) error {
	for _, cache := range e.caches {
		if !cache.IsOutOfRange(clock.Number) {
			continue
		}
		// FIXME(abourget): here we have no guarantee that we only have written
		// fINALIZED blocks in the cache.  IN fact, we could very well have received
		// New blocks in the live.
		// Is that a problem? If we have written blocks in the cache that are not
		// in the range we're about to write, they'll have been written in the future
		// (for a cache of 0-100, we might have written 105 and 106).
		// Will we easily find those back??
		// Perhaps we should only write caches for finalized blocks, and purge
		// those block IDs that have not been marked as FINAL from storage.
		// In file.go:188 we save the `.kv` indistinctly.
		if err := e.flushCache(cache); err != nil {
			return fmt.Errorf("flushing output cache %s: %w", cache.ModuleName, err)
		}

	}
	return nil
}

func (e *Engine) HandleUndo(clock *pbsubstreams.Clock, moduleName string) {
	if c, found := e.caches[moduleName]; found {
		c.Delete(clock.Id)
	}
}

func (e *Engine) NewExecOutput(block *bstream.Block, clock *pbsubstreams.Clock, cursor *bstream.Cursor) (execout.ExecutionOutput, error) {
	execOutMap, err := execout.NewExecOutputMap(e.blockType, block, clock)
	if err != nil {
		return nil, fmt.Errorf("setting up map: %w", err)
	}

	return &cursoredCache{
		ExecOutputMap: execOutMap,
		engine:        e,
		cursor:        cursor,
	}, nil
}

func (e *Engine) flushCache(cache *execout.File) error {
	e.logger.Debug("saving cache", zap.Object("cache", cache), zap.Int("kv_count", len(cache.outputData.Kv)))
	err := cache.Save(e.ctx)
	if err != nil {
		return fmt.Errorf("saving cache ouputs: %w", err)
	}

	if _, err := cache.LoadAtEndBlockBoundary(e.ctx); err != nil {
		return fmt.Errorf("loading cache: %w", err)
	}
	return nil
}

func (e *Engine) undoCaches(blockRef bstream.BlockRef) error {
	for _, cache := range e.caches {
		cache.Delete(blockRef.ID())
	}
	return nil
}

func (e *Engine) get(moduleName string, clock *pbsubstreams.Clock) ([]byte, bool, error) {
	cache, found := e.caches[moduleName]
	if !found {
		return nil, false, fmt.Errorf("cache %q not found in: %v", moduleName, e.caches)
	}

	// TODO(abourget): it's none of the business of the Engine to know
	// whether the `cache` should initialized itself, or load whatever
	// the first call to `Get()` should manage that.
	if !cache.IsInitialized() {
		if _, err := cache.LoadAtBlock(e.ctx, clock.Number); err != nil {
			return nil, false, fmt.Errorf("unable to load cache %q at block %d: %w", moduleName, clock.Number, err)
		}
	}

	data, found := cache.Get(clock)
	return data, found, nil
}

func (e *Engine) set(moduleName string, data []byte, clock *pbsubstreams.Clock, cursor string) error {
	cache, found := e.caches[moduleName]
	if !found {
		return fmt.Errorf("cache %q not found", moduleName)
	}

	return cache.Set(clock, cursor, data)
}

func (e *Engine) Close() error {
	for _, cache := range e.caches {
		cache.Close()
	}
	return nil
}
