package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

var zlog, _ = logging.PackageLogger("scheduler", "github.com/streamingfast/substreams/scheduler")

type Scheduler struct {
	blockRangeSizeSubRequests int

	squasher *Squasher
	strategy Strategy
	requests []*pbsubstreams.Request
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

func (s *Scheduler) Next() (*pbsubstreams.Request, error) {
	return s.strategy.GetNextRequest()
}

func (s *Scheduler) Callback(ctx context.Context, r *pbsubstreams.Request) error {
	for _, output := range r.GetOutputModules() {
		err := s.squasher.Squash(ctx, output, &block.Range{
			StartBlock:        uint64(r.StartBlockNum),
			ExclusiveEndBlock: r.StopBlockNum,
		})

		if err != nil {
			return fmt.Errorf("squashing: %w", err)
		}
	}
	return nil
}

func (s *Scheduler) RequestCount() int {
	return s.strategy.RequestCount()
}
