package wasm

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"
)

// Registry from Substreams's perspective is a singleton that is
// reused across requests, from which we instantiate Modules (wasm code provided by the users)
// and from which we instantiate Instances (one for each executions within each blocks).
type Registry struct {
	Extensions   map[string]map[string]WASMExtension
	maxFuel      uint64
	runtimeStack ModuleFactory
}

func (r *Registry) registerWASMExtension(namespace string, importName string, ext WASMExtension) {
	if namespace == "state" {
		panic("cannot extend 'state' wasm namespace")
	}
	if namespace == "env" {
		panic("cannot extend 'env' wasm namespace")
	}
	if namespace == "logger" {
		panic("cannot extend 'logger' wasm namespace")
	}

	if r.Extensions == nil {
		r.Extensions = map[string]map[string]WASMExtension{}
	}
	if r.Extensions[namespace] == nil {
		r.Extensions[namespace] = map[string]WASMExtension{}
	}
	if r.Extensions[namespace][importName] != nil {
		panic(fmt.Sprintf("wasm extension namespace %q function %q already defined", namespace, importName))
	}
	r.Extensions[namespace][importName] = ext
}

func (r *Registry) NewModule(ctx context.Context, wasmCode []byte) (Module, error) {
	return r.runtimeStack.NewModule(ctx, wasmCode, r)
}

func NewRegistry(extensions []WASMExtensioner, maxFuel uint64) *Registry {
	r := &Registry{
		maxFuel: maxFuel,
	}
	for _, ext := range extensions {
		for ns, exts := range ext.WASMExtensions() {
			for name, ext := range exts {
				r.registerWASMExtension(ns, name, ext)
			}
		}
	}
	runtimeName := "wazero"
	runtime := runtimes[runtimeName]
	//fmt.Println("RUNTIME CHOSEN", runtimeName, runtime)
	if selectRuntime := os.Getenv("SUBSTREAMS_WASM_RUNTIME"); selectRuntime != "" {
		selectedRuntime := runtimes[selectRuntime]
		if selectedRuntime == nil {
			zlog.Warn("CANNOT FIND WASM RUNTIME SPECIFIED IN SUBSTREAMS_WASM_RUNTIME ENV VAR, USING DEFAULT", zap.String("runtime", runtimeName))
		} else {
			runtimeName = selectRuntime
			runtime = selectedRuntime
			zlog.Warn("USING WASM RUNTIME SPECIFIED IN SUBSTREAMS_WASM_RUNTIME ENV VAR", zap.String("runtime", runtimeName))
		}
	}
	r.runtimeStack = runtime
	return r
}
