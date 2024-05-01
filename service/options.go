package service

import (
	"github.com/streamingfast/substreams/wasm"
)

type anyTierService interface{}

type Option func(anyTierService)

func WithModuleExecutionTracing() Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.runtimeConfig.ModuleExecutionTracing = true
		case *Tier2Service:
			s.moduleExecutionTracing = true
		}
	}
}

func WithWASMExtensioner(ext wasm.WASMExtensioner) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			exts, err := ext.WASMExtensions(ext.Params())
			if err != nil {
				panic(err)
			}

			s.wasmExtensions = exts
			s.wasmParams = ext.Params()
		case *Tier2Service:
			s.wasmExtensions = ext.WASMExtensions
		}
	}
}

func WithMaxConcurrentRequests(max uint64) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			// not used
		case *Tier2Service:
			s.maxConcurrentRequests = max
		}
	}

}

func WithReadinessFunc(f func(bool)) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			// not used
		case *Tier2Service:
			s.setReadyFunc = f
		}
	}
}
