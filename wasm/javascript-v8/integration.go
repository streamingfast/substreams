package v8

import (
	"github.com/streamingfast/substreams/wasm"
)

func init() {
	wasm.RegisterModuleFactory("javascript-v8", wasm.ModuleFactoryFunc(NewModule))
}
