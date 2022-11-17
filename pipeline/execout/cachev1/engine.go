package cachev1

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
	"github.com/streamingfast/substreams/service/config"
	"go.uber.org/zap"
)

var cacheFilenameRegex *regexp.Regexp

func init() {
	cacheFilenameRegex = regexp.MustCompile(`([\d]+)-([\d]+)\.output`)
}

type Engine struct {
	ctx           context.Context
	blockType     string
	caches        map[string]*OutputCache
	runtimeConfig config.RuntimeConfig
	logger        *zap.Logger
	wg            *sync.WaitGroup
}

func NewEngine(runtimeConfig config.RuntimeConfig, blockType string, logger *zap.Logger) (execout.CacheEngine, error) {
	e := &Engine{
		ctx:           context.Background(),
		runtimeConfig: runtimeConfig,
		caches:        make(map[string]*OutputCache),
		logger:        logger,
		wg:            &sync.WaitGroup{},
		blockType:     blockType,
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

func (e *Engine) EndOfStream(isSubrequest bool, outputModules map[string]bool) error {
	for _, cache := range e.caches {
		if isSubrequest && outputModules[cache.moduleName] {
			continue
		}
		if err := e.flushCache(cache); err != nil {
			return fmt.Errorf("flushing output cache %s: %w", cache.moduleName, err)
		}
	}
	return nil
}

func (e *Engine) HandleFinal(clock *pbsubstreams.Clock) error {
	for _, cache := range e.caches {
		if !cache.isOutOfRange(clock.Number) {
			continue
		}
		if err := e.flushCache(cache); err != nil {
			return fmt.Errorf("flushing output cache %s: %w", cache.moduleName, err)
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

	return &ExecOutputCache{
		ExecOutputMap: execOutMap,
		engine:        e,
		cursor:        cursor,
	}, nil
}

func (e *Engine) flushCache(cache *OutputCache) error {
	e.logger.Debug("saving cache", zap.Object("cache", cache), zap.Int("kv_count", len(cache.outputData.Kv)))
	err := cache.save(e.ctx, cache.currentFilename())
	if err != nil {
		return fmt.Errorf("saving cache ouputs: %w", err)
	}

	if _, err := cache.LoadAtBlock(e.ctx, cache.currentBlockRange.ExclusiveEndBlock); err != nil {
		return fmt.Errorf("loading cache outputs  at blocks %d: %w", cache.currentBlockRange.ExclusiveEndBlock, err)
	}
	return nil
}

func (e *Engine) undoCaches(blockRef bstream.BlockRef) error {
	for _, cache := range e.caches {
		cache.Delete(blockRef.ID())
	}
	return nil
}

func (e *Engine) registerCache(moduleName, moduleHash string) error {
	e.logger.Debug("registering modules", zap.String("module_name", moduleName))

	if _, found := e.caches[moduleName]; found {
		return fmt.Errorf("cache alreayd registered: %q", moduleName)
	}

	moduleStore, err := e.runtimeConfig.BaseObjectStore.SubStore(fmt.Sprintf("%s/outputs", moduleHash))
	if err != nil {
		return fmt.Errorf("failed createing substore: %w", err)
	}

	e.caches[moduleName] = NewOutputCache(moduleName, moduleStore, e.runtimeConfig.StoreSnapshotsSaveInterval, e.logger, e.wg)

	return nil
}

func (e *Engine) get(moduleName string, clock *pbsubstreams.Clock) ([]byte, bool, error) {
	cache, found := e.caches[moduleName]
	if !found {
		return nil, false, fmt.Errorf("cache %q not found in: %v", moduleName, e.caches)
	}
	if !cache.initialized {
		if _, err := cache.LoadAtBlock(e.ctx, clock.Number); err != nil {
			return nil, false, fmt.Errorf("unable to load cache %q at block %d: %w", moduleName, clock.Number, err)
		}
		cache.initialized = true
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
	e.wg.Wait()
	return nil
}
