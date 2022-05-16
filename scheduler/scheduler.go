package scheduler

import (
	"context"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/squasher"
	"github.com/streamingfast/substreams/state"
)

type Scheduler struct {
	ctx           context.Context
	ctxCancelFunc context.CancelFunc

	squasher *squasher.Squasher
	requests []*pbsubstreams.Request
}

func NewScheduler(ctx context.Context, request *pbsubstreams.Request, builders map[string]*state.Builder, squasher *squasher.Squasher) *Scheduler {
	return nil
}

func (s *Scheduler) callback(r *pbsubstreams.Request, err error) {
	if err != nil {
		s.ctxCancelFunc()
	}

	for _, output := range r.GetOutputModules() {
		_ = s.squasher.Squash(output, &block.Range{
			StartBlock:        uint64(r.StartBlockNum),
			ExclusiveEndBlock: r.StopBlockNum,
		})
	}

}

func (s *Scheduler) getNextRequest() *pbsubstreams.Request {
	/// super smart algo here:
	return nil
}

func (s *Scheduler) Next(f func(request *pbsubstreams.Request, callback func(r *pbsubstreams.Request, err error))) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
		// go!
	}

	var request *pbsubstreams.Request

	f(request, s.callback)

	return nil
}
