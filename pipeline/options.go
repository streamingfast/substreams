package pipeline

import (
	"context"

	"github.com/streamingfast/substreams"
)

type PipelineOptioner interface {
	PipelineOptions(ctx context.Context, startBlock, stopBlock uint64, traceID string) []Option
}

type Option func(p *Pipeline)

func WithPreBlockHook(f substreams.BlockHook) Option {
	return func(p *Pipeline) {
		p.preBlockHooks = append(p.preBlockHooks, f)
	}
}

// WithPreFirstBlockDataHook functions will be called before we send the first 'BlockScopedData'
// to the consumer
func WithPreFirstBlockDataHook(f substreams.BlockHook) Option {
	return func(p *Pipeline) {
		p.preFirstBlockDataHooks = append(p.preBlockHooks, f)
	}
}

func WithPostBlockHook(f substreams.BlockHook) Option {
	return func(p *Pipeline) {
		p.postBlockHooks = append(p.postBlockHooks, f)
	}
}

func WithPostJobHook(f substreams.PostJobHook) Option {
	return func(p *Pipeline) {
		p.postJobHooks = append(p.postJobHooks, f)
	}
}
