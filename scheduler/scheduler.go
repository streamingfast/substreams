package scheduler

import (
	"context"
	"fmt"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/squasher"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
	"io"
)

var zlog, _ = logging.PackageLogger("scheduler", "github.com/streamingfast/substreams/scheduler")

type Scheduler struct {
	ctx           context.Context
	ctxCancelFunc context.CancelFunc

	squasher *squasher.Squasher
	requests []*pbsubstreams.Request
}

func NewScheduler(ctx context.Context, request *pbsubstreams.Request, builders []*state.Builder, upToBlockNum uint64, squasher *squasher.Squasher) (*Scheduler, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &Scheduler{
		ctx:           ctx,
		ctxCancelFunc: cancel,
		squasher:      squasher,
		requests:      []*pbsubstreams.Request{},
	}

	for _, builder := range builders {
		zlog.Debug("builders", zap.String("builder", builder.Name))
		zlog.Debug("up to block num", zap.Uint64("up_to_block_num", upToBlockNum))
		if upToBlockNum == builder.ModuleStartBlock {
			continue // nothing to synchronize
		}

		endBlock := upToBlockNum
		info, err := builder.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting builder info: %w", err)
		}

		lastExclusiveEndBlock := info.LastKVSavedBlock
		zlog.Debug("got info", zap.Object("builder", builder), zap.Uint64("up_to_block", upToBlockNum), zap.Uint64("end_block", lastExclusiveEndBlock))
		if upToBlockNum <= lastExclusiveEndBlock {
			zlog.Debug("no request created", zap.Uint64("up_to_block", upToBlockNum), zap.Uint64("last_exclusive_end_block", lastExclusiveEndBlock))
			continue // not sure if we should pop here
		}

		reqStartBlock := lastExclusiveEndBlock
		if reqStartBlock == 0 {
			reqStartBlock = builder.ModuleStartBlock
		}

		req := createRequest(reqStartBlock, endBlock, builder.Name, request.ForkSteps, request.IrreversibilityCondition, request.Manifest)
		s.requests = append(s.requests, req)
	}

	return s, nil
}

func (s *Scheduler) Next(f func(request *pbsubstreams.Request, callback func(r *pbsubstreams.Request, err error))) error {
	request, err := s.getNextRequest()
	if err != nil {
		return io.EOF
	}

	zlog.Debug("request", zap.Int64("start_block", request.StartBlockNum), zap.Uint64("stop_block", request.StopBlockNum), zap.Strings("stores", request.OutputModules))
	f(request, s.callback)

	return nil
}

func (s *Scheduler) callback(r *pbsubstreams.Request, err error) {
	if err != nil {
		s.ctxCancelFunc()
		return
	}

	for _, output := range r.GetOutputModules() {
		err = s.squasher.Squash(s.ctx, output, &block.Range{
			StartBlock:        uint64(r.StartBlockNum),
			ExclusiveEndBlock: r.StopBlockNum,
		})

		if err != nil {
			zlog.Error("squashing output", zap.String("output", output), zap.Error(err))
			s.ctxCancelFunc()
			return
		}
	}

}

func (s *Scheduler) getNextRequest() (*pbsubstreams.Request, error) {
	if len(s.requests) == 0 {
		return nil, fmt.Errorf("no requests to fetch")
	}

	var request *pbsubstreams.Request
	request, s.requests = s.requests[len(s.requests)-1], s.requests[:len(s.requests)-1]

	return request, nil
}

func createRequest(
	startBlock, stopBlock uint64,
	outputModuleName string,
	forkSteps []pbsubstreams.ForkStep,
	irreversibilityCondition string,
	manifest *pbsubstreams.Manifest,
) *pbsubstreams.Request {
	return &pbsubstreams.Request{
		StartBlockNum:            int64(startBlock),
		StopBlockNum:             stopBlock,
		ForkSteps:                forkSteps,
		IrreversibilityCondition: irreversibilityCondition,
		Manifest:                 manifest,
		OutputModules:            []string{outputModuleName},
	}
}
