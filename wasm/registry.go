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
	Extensions           map[string]map[string]WASMExtension
	maxFuel              uint64
	runtimeStack         ModuleFactory
	instanceCacheEnabled bool
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
func (r *Registry) MaxFuel() uint64            { return r.maxFuel }
func (r *Registry) InstanceCacheEnabled() bool { return r.instanceCacheEnabled }

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

	if cache := os.Getenv("SUBSTREAMS_WASM_CACHE_ENABLED"); cache != "" {
		r.instanceCacheEnabled = cache != "false"
	}
	cacheField := zap.Bool("cache_enabled", r.instanceCacheEnabled)

	runtimeName := "wasmtime" // fallback engine
	runtime := runtimes[runtimeName]
	//fmt.Println("RUNTIME CHOSEN", runtimeName, runtime)
	if selectRuntime := os.Getenv("SUBSTREAMS_WASM_RUNTIME"); selectRuntime != "" {
		selectedRuntime := runtimes[selectRuntime]
		if selectedRuntime == nil {
			panic(fmt.Errorf("could not find wasm runtime specified by `SUBSTREAMS_WASM_RUNTIME` env var: %q", selectRuntime))
		} else {
			runtimeName = selectRuntime
			runtime = selectedRuntime
			zlog.Info("using wasm runtime specified by env var", zap.String("runtime", runtimeName), cacheField)
		}
	} else {
		zlog.Info("using default wasm runtime", zap.String("runtime", runtimeName), cacheField)
	}
	r.runtimeStack = runtime

	return r
}
