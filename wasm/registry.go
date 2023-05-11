package wasm

import "fmt"

// Registry from Substreams's perspective is a singleton that is
// reused across requests, from which we instantiate Modules (wasm code provided by the users)
// and from which we instantiate Instances (one for each executions within each blocks).
type Registry struct {
	extensions map[string]map[string]WASMExtension
	maxFuel    uint64
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

	if r.extensions == nil {
		r.extensions = map[string]map[string]WASMExtension{}
	}
	if r.extensions[namespace] == nil {
		r.extensions[namespace] = map[string]WASMExtension{}
	}
	if r.extensions[namespace][importName] != nil {
		panic(fmt.Sprintf("wasm extension namespace %q function %q already defined", namespace, importName))
	}
	r.extensions[namespace][importName] = ext
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
	return r
}
