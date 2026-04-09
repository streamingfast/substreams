package cache

import (
	"sync"
	"time"

	"github.com/streamingfast/substreams/wasm"
)

type ModuleCache struct {
	sync.Mutex
	modules map[string]cacheEntry
}

type cacheEntry struct {
	mod       wasm.Module
	fetchedAt time.Time
}

func NewModuleCache(evictionInterval time.Duration, ttl time.Duration) *ModuleCache {
	c := &ModuleCache{
		modules: make(map[string]cacheEntry),
	}

	go func() {
		ticker := time.NewTicker(evictionInterval)
		defer ticker.Stop()

		for range ticker.C {
			c.Lock()
			cutoff := time.Now().Add(-ttl)
			for hash, entry := range c.modules {
				if entry.fetchedAt.Before(cutoff) {
					delete(c.modules, hash)
				}
			}
			c.Unlock()
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
