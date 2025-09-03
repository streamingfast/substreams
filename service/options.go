package service

import (
	"time"

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

func WithBlockExecutionTimeout(timeout time.Duration) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.blockExecutionTimeout = timeout
		case *Tier2Service:
			s.blockExecutionTimeout = timeout
		}
	}
}

// Tier2 will completely bail out if a segment execution takes longer than the this.
func WithSegmentExecutionTimeout(timeout time.Duration) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			// not used
		case *Tier2Service:
			s.segmentExecutionTimeout = timeout
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
			s.appSetIsReadyState = f
		}
	}
}
