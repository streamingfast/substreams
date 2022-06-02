package orchestrator

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/worker"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type Scheduler struct {
	blockRangeSizeSubRequests int

	workerPool *worker.Pool
	respFunc   substreams.ResponseFunc

	squasher       *Squasher
	requestsStream <-chan *pbsubstreams.Request
	requests       []*pbsubstreams.Request
}

func NewScheduler(ctx context.Context, strategy Strategy, squasher *Squasher, workerPool *worker.Pool, respFunc substreams.ResponseFunc, blockRangeSizeSubRequests int) (*Scheduler, error) {
	s := &Scheduler{
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		squasher:                  squasher,
		requestsStream:            GetRequestStream(ctx, strategy),
		requests:                  []*pbsubstreams.Request{},
		workerPool:                workerPool,
		respFunc:                  respFunc,
	}

	return s, nil
}

func (s *Scheduler) Next() (*pbsubstreams.Request, error) {
	zlog.Debug("Getting a next job from scheduler", zap.Int("requests_stream", len(s.requestsStream)))
	request, ok := <-s.requestsStream
	if !ok {
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

func (s *Scheduler) Launch(ctx context.Context, result chan error) (out chan error) {
	out = make(chan error, 1) // FIXME: not used, not necessary?
	go func() {
		if err := s.doLaunch(ctx, result); err != nil {
			out <- err
		}
	}()
	return
}

func (s *Scheduler) doLaunch(ctx context.Context, result chan error) error {
	for {
		req, err := s.Next()
		if err == io.EOF {
			zlog.Debug("scheduler do launch EOF")
			break
		}
		if err != nil {
			return err
		}

		job := &worker.Job{
			Request: req,
		}

		zlog.Info("scheduling job", zap.Object("job", job))

		start := time.Now()
		jobWorker := s.workerPool.Borrow()
		zlog.Debug("got worker", zap.Object("job", job), zap.Duration("in", time.Since(start)))

		select {
		case <-ctx.Done():
			zlog.Info("synchronize stores quit on cancel context")
			return nil
		default:
		}

		go func() {
			result <- s.runSingleJob(ctx, jobWorker, job)
		}()
	}
	return nil
}

func (s *Scheduler) runSingleJob(ctx context.Context, jobWorker *worker.Worker, job *worker.Job) error {
	err := derr.RetryContext(ctx, 2, func(ctx context.Context) error {
		return jobWorker.Run(ctx, job, s.respFunc)
	})
	s.workerPool.ReturnWorker(jobWorker)
	if err != nil {
		return err
	}

	if err = s.Callback(ctx, job.Request); err != nil {
		return fmt.Errorf("calling back scheduler: %w", err)
	}
	return nil

}
