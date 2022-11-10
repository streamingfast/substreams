package cache

import (
	"fmt"

	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/bstream"
)

type cursoredCache struct {
	*execout.ExecOutputMap

	engine *Engine
	cursor *bstream.Cursor
}

// assert_test_store_delete_prefix
func (e *cursoredCache) Get(moduleName string) (value []byte, cached bool, err error) {
	val, _, err := e.ExecOutputMap.Get(moduleName)
	if err != nil && err != execout.NotFound {
		return nil, false, fmt.Errorf("get from memory: %w", err)
	}
	if err == nil {
		return val, false, nil
	}

	val, found, err := e.engine.get(moduleName, e.Clock())
	if err != nil {
		return nil, false, fmt.Errorf("get from cache: %w", err)
	}
	if found {
		return val, true, nil
	}

	return nil, false, execout.NotFound
}

func (e *cursoredCache) Set(moduleName string, value []byte) (err error) {
	if err := e.ExecOutputMap.Set(moduleName, value); err != nil {
		return err
	}
	return e.engine.set(moduleName, value, e.Clock(), e.cursor.ToOpaque())
}
