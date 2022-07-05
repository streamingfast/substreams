package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
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

func (s *Scheduler) Launch(ctx context.Context, result chan error) {
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
			case result <- s.runSingleJob(ctx, jobWorker, job):
			case <-ctx.Done():
			}
		}()
	}
}

func (s *Scheduler) runSingleJob(ctx context.Context, jobWorker *Worker, job *Job) error {
	var partialsWritten []*block.Range
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		var err error
		partialsWritten, err = jobWorker.Run(ctx, job, s.respFunc)
		if err != nil {
			return err
		}
		return nil
	})
	s.workerPool.ReturnWorker(jobWorker)
	if err != nil {
		return err
	}

	if err = s.squasher.Squash(job.moduleName, partialsWritten); err != nil {
		return fmt.Errorf("squashing: %w", err)
	}
	return nil
}
