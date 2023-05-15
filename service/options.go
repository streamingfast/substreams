package service

import (
	"github.com/streamingfast/substreams/pipeline"
	"github.com/streamingfast/substreams/wasm"
)

type anyTierService interface{}

type Option func(anyTierService)

func WithWASMExtension(ext wasm.WASMExtensioner) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.wasmExtensions = append(s.wasmExtensions, ext)
		case *Tier2Service:
			s.wasmExtensions = append(s.wasmExtensions, ext)
		}
	}
}

// WithPipelineOptions is used to configure pipeline options for
// consumer outside of the substreams library itself, for example
// in chain specific Firehose implementations.
func WithPipelineOptions(f pipeline.PipelineOptioner) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.pipelineOptions = append(s.pipelineOptions, f)
		case *Tier2Service:
			s.pipelineOptions = append(s.pipelineOptions, f)
		}
	}
}

func WithCacheSaveInterval(block uint64) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.runtimeConfig.CacheSaveInterval = block
		case *Tier2Service:
			s.runtimeConfig.CacheSaveInterval = block
		}
	}

}

func WithRequestStats() Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.runtimeConfig.WithRequestStats = true
		case *Tier2Service:
			s.runtimeConfig.WithRequestStats = true
		}
	}
}

func WithMaxWasmFuelPerBlockModule(maxFuel uint64) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.runtimeConfig.MaxWasmFuel = maxFuel
		case *Tier2Service:
			s.runtimeConfig.MaxWasmFuel = maxFuel
		}
	}
}
