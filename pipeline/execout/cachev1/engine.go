package cachev1

import (
	"context"
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
	"go.uber.org/zap"
	"regexp"
)

var cacheFilenameRegex *regexp.Regexp

func init() {
	cacheFilenameRegex = regexp.MustCompile(`([\d]+)-([\d]+)\.output`)
}

type Engine struct {
	ctx               context.Context
	caches            map[string]*OutputCacheState
	SaveBlockInterval uint64
	baseCacheStore    dstore.Store
	logger            *zap.Logger
}

type OutputCacheState struct {
	c           *OutputCache
	initialized bool
}

func NewEngine(ctx context.Context, saveBlockInterval uint64, baseCacheStore dstore.Store, logger *zap.Logger) (execout.CacheEngine, error) {
	e := &Engine{
		ctx:               ctx,
		caches:            make(map[string]*OutputCacheState),
		SaveBlockInterval: saveBlockInterval,
		baseCacheStore:    baseCacheStore,
		logger:            logger,
	}
	return e, nil
}
func (e *Engine) Init(modules *manifest.ModuleHashes) error {
	return modules.Iter(func(hash, name string) error {
		if err := e.registerCache(name, hash); err != nil {
			return fmt.Errorf("failed to register chache for module %q: %w", name, err)
		}
		return nil
	})
}
func (e *Engine) NewBlock(blockRef bstream.BlockRef, step bstream.StepType) error {
	if step.Matches(bstream.StepIrreversible) {
		return e.flushCaches(blockRef)
	}
	if step.Matches(bstream.StepUndo) {
		e.undoCaches(blockRef)
		return nil
	}
	return nil
}
func (e *Engine) flushCaches(blockRef bstream.BlockRef) error {
	for name, cache := range e.caches {
		if cache.c.IsOutOfRange(blockRef) {
			e.logger.Debug("saving cache", zap.Object("cache", cache.c))
			if err := cache.c.save(e.ctx, cache.c.currentFilename()); err != nil {
				return fmt.Errorf("save: saving outpust or module kv %s: %w", name, err)
			}

			if _, err := cache.c.LoadAtBlock(e.ctx, cache.c.currentBlockRange.ExclusiveEndBlock); err != nil {
				return fmt.Errorf("loading blocks %d for module kv %s: %w", cache.c.currentBlockRange.ExclusiveEndBlock, cache.c.moduleName, err)
			}
		}
	}
	return nil
}

func (e *Engine) undoCaches(blockRef bstream.BlockRef) error {
	for _, cache := range e.caches {
		cache.c.Delete(blockRef.ID())
	}
	return nil
}

func (e *Engine) UndoBlock(moduleName string, blockID string) {
	if outputCache, found := e.caches[moduleName]; found {
		outputCache.c.Delete(blockID)
	}
}

func (e *Engine) registerCache(moduleName, moduleHash string) error {
	e.logger.Debug("registering modules", zap.String("module_name", moduleName))

	if _, found := e.caches[moduleName]; found {
		return fmt.Errorf("cache alreayd registered: %q", moduleName)
	}

	moduleStore, err := e.baseCacheStore.SubStore(fmt.Sprintf("%s/outputs", moduleHash))
	if err != nil {
		return fmt.Errorf("failed createing substore: %w", err)
	}

	e.caches[moduleName] = &OutputCacheState{
		c:           NewOutputCache(moduleName, moduleStore, e.SaveBlockInterval, e.logger),
		initialized: false,
	}
	return nil
}

func (e *Engine) get(moduleName string, clock *pbsubstreams.Clock) ([]byte, bool, error) {
	cache, found := e.caches[moduleName]
	if !found {
		return nil, false, fmt.Errorf("cache %q not found", moduleName)
	}
	if !cache.initialized {
		if _, err := cache.c.LoadAtBlock(e.ctx, clock.Number); err != nil {
			return nil, false, fmt.Errorf("unable to load cache %q at block %d: %w", moduleName, clock.Number, err)
		}
		cache.initialized = true
	}

	data, found := cache.c.Get(clock)
	return data, found, nil
}

func (e *Engine) set(moduleName string, data []byte, clock *pbsubstreams.Clock, cursor string) error {
	cache, found := e.caches[moduleName]
	if !found {
		return fmt.Errorf("cache %q not found", moduleName)
	}

	return cache.c.Set(clock, cursor, data)
}
