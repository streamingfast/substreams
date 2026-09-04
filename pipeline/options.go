package pipeline

import (
	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/storage/execout"
)

type Option func(p *Pipeline)

func WithPreBlockHook(f substreams.BlockHook) Option {
	return func(p *Pipeline) {
		p.preBlockHooks = append(p.preBlockHooks, f)
	}
}

// WithPendingUndoMessage allows sending a message right before we send the first 'BlockScopedData'
func WithPendingUndoMessage(msg *pbsubstreamsrpc.Response) Option {
	return func(p *Pipeline) {
		p.pendingUndoMessage = msg
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

func WithFinalBlocksOnly() Option {
	return func(p *Pipeline) {
		p.finalBlocksOnly = true
	}
}

func WithHeadBlockGetter(getter func() (uint64, error)) Option {
	return func(p *Pipeline) {
		p.getHeadBlockNum = getter
	}
}

func WithHighestStage(stage uint32) Option {
	return func(p *Pipeline) {
		s := int(stage)
		p.highestStage = &s
	}
}

// WithExecOutPrefetch bounds how far ahead tier1 downloads cached execution
// output files while streaming them to the client.
func WithExecOutPrefetch(cfg execout.PrefetchConfig) Option {
	return func(p *Pipeline) {
		p.execOutPrefetch = cfg
	}
}
