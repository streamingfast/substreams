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
	runtimeStacks        map[string]ModuleFactory
	instanceCacheEnabled bool
}

func (r *Registry) RuntimeStack(wasmCodeType string) ModuleFactory {
	if fac, ok := r.runtimeStacks[wasmCodeType]; ok {
		return fac
	}
	return r.runtimeStacks["default"]
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
func (r *Registry) InstanceCacheEnabled() bool { return r.instanceCacheEnabled }

func (r *Registry) NewModule(ctx context.Context, wasmCode []byte, wasmCodeType string) (Module, error) {
	return r.RuntimeStack(wasmCodeType).NewModule(ctx, wasmCode, wasmCodeType, r)
}

func NewRegistry(extensions map[string]map[string]WASMExtension) *Registry {

	defaultRuntime := "wasmtime"

	if selectRuntime := os.Getenv("SUBSTREAMS_WASM_RUNTIME"); selectRuntime != "" {
		selectedRuntime := runtimes[selectRuntime]
		if selectedRuntime == nil {
			panic(fmt.Errorf("could not find wasm runtime specified by `SUBSTREAMS_WASM_RUNTIME` env var: %q", selectRuntime))
		}
		defaultRuntime = selectRuntime
	} else {
		zlog.Debug("using default wasm runtime", zap.String("runtime", defaultRuntime))
	}

	r := &Registry{}

	for ns, exts := range extensions {
		for name, ext := range exts {
			r.registerWASMExtension(ns, name, ext)
		}
	}

	if cache := os.Getenv("SUBSTREAMS_WASM_CACHE_ENABLED"); cache == "true" {
		zlog.Warn("running with WASM cache because SUBSTREAMS_WASM_CACHE_ENABLED variable was set -- this will produce non-deterministic output and poison your cache. Never use the WASM cache in production.")
		r.instanceCacheEnabled = true
	}

	r.runtimeStacks = map[string]ModuleFactory{
		"default":                         runtimes[defaultRuntime],
		"wasip1/tinygo-v1":                runtimes["wazero"], // only wazero supports tinygo at the moment
		"wasm/rust-v1":                    runtimes[defaultRuntime],
		"javascript/v8":                   runtimes["v8"],
		"wasm/rust-v1+wasm-bindgen-shims": runtimes[defaultRuntime],
	}

	return r
}
