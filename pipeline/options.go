package pipeline

import (
	"context"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/metrics"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type PipelineOptioner interface {
	PipelineOptions(ctx context.Context, request *pbsubstreams.Request) []Option
}

type Option func(p *Pipeline)

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

func WithSyncBlockRangeRestriction(maxRangeSize uint64) Option {
	return func(p *Pipeline) {
		p.maxStoreSyncRangeSize = maxRangeSize
	}
}

func WithRequestStats(stats metrics.Stats) Option {
	return func(p *Pipeline) {
		p.stats = stats
	}
}
