package cache

import (
	"fmt"

	"github.com/streamingfast/substreams/storage/execout"
)

// DELETE ME, no need any more.. replaced by other constructs.

type cursoredCache struct {
	*execout.ExecOutputBuffer

	engine *Engine
	cursor string
}

func (e *cursoredCache) Get(moduleName string) (value []byte, cached bool, err error) {
	val, _, err := e.ExecOutputBuffer.Get(moduleName)
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
	if err := e.ExecOutputBuffer.Set(moduleName, value); err != nil {
		return err
	}
	return e.engine.set(moduleName, value, e.Clock(), e.cursor)
}
