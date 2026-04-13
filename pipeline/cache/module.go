package cache

import (
	"context"
	"sync"
	"time"

	"github.com/streamingfast/substreams/wasm"
	"go.uber.org/zap"
)

type ModuleCache struct {
	sync.Mutex
	modules map[string]cacheEntry
}

type cacheEntry struct {
	mod       wasm.Module
	fetchedAt time.Time
}

func NewModuleCache(ctx context.Context, evictionInterval time.Duration, ttl time.Duration, logger *zap.Logger) *ModuleCache {
	logger = logger.Named("module-cache")
	c := &ModuleCache{
		modules: make(map[string]cacheEntry),
	}

	go func() {
		ticker := time.NewTicker(evictionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.Lock()
				cutoff := time.Now().Add(-ttl)
				for hash, entry := range c.modules {
					if entry.fetchedAt.Before(cutoff) {
						if err := entry.mod.Close(ctx); err != nil {
							logger.Warn("closing wasm module", zap.String("hash", hash), zap.Error(err))
						}

						delete(c.modules, hash)
					}
				}
				c.Unlock()
			}
		}
	}()

	return c
}

func (c *ModuleCache) Get(hash string) (wasm.Module, bool) {
	c.Lock()
	defer c.Unlock()
	entry, ok := c.modules[hash]
	if !ok {
		return nil, false
	}
	return entry.mod, true
}

func (c *ModuleCache) Set(hash string, module wasm.Module) {
	c.Lock()
	defer c.Unlock()
	c.modules[hash] = cacheEntry{mod: module, fetchedAt: time.Now()}
}
