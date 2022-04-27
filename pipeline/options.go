package pipeline

import "github.com/streamingfast/substreams"

type PipelineOptioner interface {
	PipelineOptions(requestedStartBlock uint64, stopBlock uint64) []Option
}

type Option func(p *Pipeline)

func WithPartialMode() Option {
	return func(p *Pipeline) {
		p.partialMode = true
	}
}

func WithAllowInvalidState() Option {
	return func(p *Pipeline) {
		p.allowInvalidState = true
	}
}

func WithPreBlockHook(f substreams.BlockHook) Option {
	return func(p *Pipeline) {
		p.preBlockHooks = append(p.preBlockHooks, f)
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
