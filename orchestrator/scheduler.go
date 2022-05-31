package orchestrator

import (
	"context"
	"fmt"
	"io"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type Scheduler struct {
	blockRangeSizeSubRequests int

	squasher       *Squasher
	requestsStream <-chan *pbsubstreams.Request
	requests       []*pbsubstreams.Request
}

func NewScheduler(ctx context.Context, strategy Strategy, squasher *Squasher, blockRangeSizeSubRequests int) (*Scheduler, error) {
	s := &Scheduler{
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		squasher:                  squasher,
		requestsStream:            GetRequestStream(ctx, strategy),
		requests:                  []*pbsubstreams.Request{},
	}

	return s, nil
}

func (s *Scheduler) Next() (*pbsubstreams.Request, error) {
	request, alive := <-s.requestsStream
	if !alive {
		return nil, io.EOF
	}

	return request, nil
}

func (s *Scheduler) Callback(ctx context.Context, outgoingReq *pbsubstreams.Request) error {
	for _, output := range outgoingReq.GetOutputModules() {
		// FIXME(abourget): why call Squash on non-store modules? Oh,
		// but the orchestrator far far away won't do that
		// anyway... hermm ok..
		err := s.squasher.Squash(ctx, output, &block.Range{
			StartBlock:        uint64(outgoingReq.StartBlockNum),
			ExclusiveEndBlock: outgoingReq.StopBlockNum,
		})

		if err != nil {
			return fmt.Errorf("squashing: %w", err)
		}
	}
	return nil
}
