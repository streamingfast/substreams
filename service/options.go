package service

import (
	"os"
	"path/filepath"
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

func WithFoundationalStoreEndpoints(endpoints map[string]string) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			// not used
		case *Tier2Service:
			s.foundationalEndpoints = endpoints
		}
	}
}

func WithStoresScratchSpace(path string) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.runtimeConfig.StoresScratchSpace = path
			sweepOrphanMmapFiles(path)
		case *Tier2Service:
			s.storesScratchSpace = path
			sweepOrphanMmapFiles(path)
		}
	}
}

// sweepOrphanMmapFiles removes bbolt files left behind by a previous process kill,
// before the service starts accepting requests. Safe to call at startup because
// no requests are in flight yet.
func sweepOrphanMmapFiles(dir string) {
	matches, _ := filepath.Glob(filepath.Join(dir, "substreams-store-*.db"))
	for _, f := range matches {
		os.Remove(f)
	}
}

func WithStoresBackend(backend string) Option {
	return func(a anyTierService) {
		switch s := a.(type) {
		case *Tier1Service:
			s.runtimeConfig.StoresBackend = backend
		case *Tier2Service:
			s.storesBackend = backend
		}
	}
}

// WithLiveBackFillerFinalBlockDelay overrides the number of blocks the live
// backfiller waits past a segment end before concluding that the merged block
// file is safely written. The default is 120 blocks.
func WithLiveBackFillerFinalBlockDelay(delay uint64) Option {
	return func(a anyTierService) {
		if s, ok := a.(*Tier1Service); ok {
			s.liveBackFillerFinalBlockDelay = delay
		}
	}
}
