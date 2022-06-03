package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams"
	"go.uber.org/zap"
)

type Scheduler struct {
	blockRangeSizeSubRequests int

	workerPool *WorkerPool
	respFunc   substreams.ResponseFunc

	squasher       *Squasher
	requestsStream <-chan *Job
	requests       []*reqChunk
}

func NewScheduler(ctx context.Context, strategy Strategy, squasher *Squasher, workerPool *WorkerPool, respFunc substreams.ResponseFunc, blockRangeSizeSubRequests int) (*Scheduler, error) {
	s := &Scheduler{
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		squasher:                  squasher,
		requestsStream:            GetRequestStream(ctx, strategy),
		requests:                  []*reqChunk{},
		workerPool:                workerPool,
		respFunc:                  respFunc,
	}

	return s, nil
}

func (s *Scheduler) Next() *Job {
	zlog.Debug("Getting a next job from scheduler", zap.Int("requests_stream", len(s.requestsStream)))
	request, ok := <-s.requestsStream
	if !ok {
		return nil
	}

	return request
}

func (s *Scheduler) Callback(ctx context.Context, job *Job) error {
	err := s.squasher.Squash(ctx, job.moduleName, job.reqChunk)
	if err != nil {
		return fmt.Errorf("squashing: %w", err)
	}
	return nil
}

func (s *Scheduler) Launch(ctx context.Context, result chan error) {
	for {
		job := s.Next()
		if job == nil {
			zlog.Debug("no more job in scheduler")
			break
		}

		zlog.Info("scheduling job", zap.Object("job", job))

		start := time.Now()
		jobWorker := s.workerPool.Borrow()
		zlog.Debug("got worker", zap.Object("job", job), zap.Duration("in", time.Since(start)))

		select {
		case <-ctx.Done():
			zlog.Info("synchronize stores quit on cancel context")
			break
		default:
		}

		go func() {
			select {
			case result <- s.runSingleJob(ctx, jobWorker, job):
			case <-ctx.Done():
			}
		}()
	}
}

func (s *Scheduler) runSingleJob(ctx context.Context, jobWorker *Worker, job *Job) error {
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		return jobWorker.Run(ctx, job, s.respFunc)
	})
	s.workerPool.ReturnWorker(jobWorker)
	if err != nil {
		return err
	}

	if err = s.Callback(ctx, job); err != nil {
		return fmt.Errorf("calling back scheduler: %w", err)
	}
	return nil

}
