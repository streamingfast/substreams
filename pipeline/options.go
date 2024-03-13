package pipeline

import (
	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
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

func WithHighestStage(stage uint32) Option {
	return func(p *Pipeline) {
		s := int(stage)
		p.highestStage = &s
	}
}
