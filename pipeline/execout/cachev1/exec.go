package cachev1

import (
	"fmt"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/pipeline/execout"
)

type ExecOutputCache struct {
	*execout.ExecOutputMap

	engine *Engine
	cursor *bstream.Cursor
}

func (e *ExecOutputCache) Get(moduleName string) (value []byte, cached bool, err error) {
	val, _, err := e.ExecOutputMap.Get(moduleName)
	if err != nil && err != execout.NotFound {
		return nil, false, fmt.Errorf("get from map: %w", err)
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

func (e *ExecOutputCache) Set(moduleName string, value []byte) (err error) {
	if err := e.ExecOutputMap.Set(moduleName, value); err != nil {
		return err
	}
	return e.engine.set(moduleName, value, e.Clock(), e.cursor.ToOpaque())
}
