package wasm

import (
	"fmt"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v4"
)

type Module struct {
	module *wasmtime.Module
	engine *wasmtime.Engine
}

func (r *Runtime) NewModule(wasmCode []byte) (*Module, error) {
	cfg := wasmtime.NewConfig()
	if r.maxFuel != 0 {
		cfg.SetConsumeFuel(true)
	}
	engine := wasmtime.NewEngineWithConfig(cfg)

	module, err := wasmtime.NewModule(engine, wasmCode)
	if err != nil {
		return nil, fmt.Errorf("creating new module: %w", err)
	}
	return &Module{
		module: module,
		engine: engine,
	}, nil
}
