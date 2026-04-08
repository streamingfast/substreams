package cache

import (
	"sync"

	"github.com/streamingfast/substreams/wasm"
)

type ModuleCache struct {
	sync.Mutex
	modules map[string]wasm.Module
}

func NewModuleCache() *ModuleCache {
	return &ModuleCache{
		modules: make(map[string]wasm.Module),
	}
}

func (c *ModuleCache) Get(hash string) (wasm.Module, bool) {
	c.Lock()
	defer c.Unlock()
	m, ok := c.modules[hash]

	return m, ok
}

func (c *ModuleCache) Set(hash string, module wasm.Module) {
	c.Lock()
	defer c.Unlock()
	c.modules[hash] = module
}
