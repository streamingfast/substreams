package orchestrator

import (
	"context"
	"io"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

var zlog, _ = logging.PackageLogger("scheduler", "github.com/streamingfast/substreams/scheduler")

type Scheduler struct {
	blockRangeSizeSubRequests int

	squasher *Squasher
	strategy Strategy
	requests []*pbsubstreams.Request
	Err      error
}

func NewScheduler(strategy Strategy, squasher *Squasher, blockRangeSizeSubRequests int) (*Scheduler, error) {
	s := &Scheduler{
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		squasher:                  squasher,
		strategy:                  strategy,
		requests:                  []*pbsubstreams.Request{},
	}

	return s, nil
}

func (s *Scheduler) Next(f func(request *pbsubstreams.Request, callback func(ctx context.Context, r *pbsubstreams.Request, err error))) error {
	request, err := s.strategy.GetNextRequest()
	if err != nil {
		return io.EOF
	}

	zlog.Debug("request", zap.Int64("start_block", request.StartBlockNum), zap.Uint64("stop_block", request.StopBlockNum), zap.Strings("stores", request.OutputModules))

	f(request, s.callback)

	return nil
}

func (s *Scheduler) callback(ctx context.Context, r *pbsubstreams.Request, err error) {
	if err != nil {
		s.Err = err
		return
	}

	for _, output := range r.GetOutputModules() {
		err = s.squasher.Squash(ctx, output, &block.Range{
			StartBlock:        uint64(r.StartBlockNum),
			ExclusiveEndBlock: r.StopBlockNum,
		})

		if err != nil {
			zlog.Error("squashing output", zap.String("output", output), zap.Error(err))
			s.Err = err
			return
		}
	}
}
