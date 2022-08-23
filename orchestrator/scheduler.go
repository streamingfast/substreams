package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/streamingfast/substreams"
	"go.uber.org/zap"
)

type Scheduler struct {
	workerPool *WorkerPool
	respFunc   substreams.ResponseFunc

	squasher      *Squasher
	availableJobs <-chan *Job
}

func NewScheduler(ctx context.Context, availableJobs chan *Job, squasher *Squasher, workerPool *WorkerPool, respFunc substreams.ResponseFunc) (*Scheduler, error) {
	s := &Scheduler{
		squasher:      squasher,
		availableJobs: availableJobs,
		workerPool:    workerPool,
		respFunc:      respFunc,
	}
	return s, nil
}

func (s *Scheduler) Launch(ctx context.Context, requestModules *pbsubstreams.Modules, result chan error) {
	for {
		zlog.Debug("getting a next job from scheduler", zap.Int("available_jobs", len(s.availableJobs)))
		job, ok := <-s.availableJobs
		if !ok {
			zlog.Debug("no more job in scheduler, or context cancelled")
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
			case result <- s.runSingleJob(ctx, jobWorker, job, requestModules):
			case <-ctx.Done():
			}
		}()
	}
}

func (s *Scheduler) runSingleJob(ctx context.Context, jobWorker *Worker, job *Job, requestModules *pbsubstreams.Modules) error {
	var partialsWritten []*block.Range
	var err error

out:
	for i := 0; uint64(i) < 3; i++ {
		partialsWritten, err = jobWorker.Run(ctx, job, s.workerPool.JobStats, requestModules, s.respFunc)

		switch err.(type) {
		case *RetryableErr:
			zlog.Debug("retryable error")
			continue
		default:
			zlog.Debug("not a retryable error")
			break out
		}
	}

	s.workerPool.ReturnWorker(jobWorker)

	if err != nil {
		return err
	}

	if partialsWritten != nil {
		if err := s.squasher.Squash(job.ModuleName, partialsWritten); err != nil {
			return fmt.Errorf("squashing: %w", err)
		}
	}

	return nil
}
