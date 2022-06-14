package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/substreams/block"

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
}

func NewScheduler(ctx context.Context, strategy *OrderedStrategy, squasher *Squasher, workerPool *WorkerPool, respFunc substreams.ResponseFunc, blockRangeSizeSubRequests int) (*Scheduler, error) {
	s := &Scheduler{
		blockRangeSizeSubRequests: blockRangeSizeSubRequests,
		squasher:                  squasher,
		requestsStream:            strategy.getRequestStream(ctx),
		workerPool:                workerPool,
		respFunc:                  respFunc,
	}
	return s, nil
}

func (s *Scheduler) Next() *Job {
	zlog.Debug("getting a next job from scheduler", zap.Int("buffered_requests", len(s.requestsStream)))
	request, ok := <-s.requestsStream
	if !ok {
		return nil
	}
	return request
}

func (s *Scheduler) Callback(ctx context.Context, job *Job, partialsChunks chunks) error {

	err := s.squasher.Squash(ctx, job.moduleName, partialsChunks)
	if err != nil {
		return fmt.Errorf("squashing: %w", err)
	}
	return nil
}

func (s *Scheduler) Launch(ctx context.Context, result chan error) {
	for {
		job := s.Next()
		if job == nil {
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

	var partialsChunks chunks
	for _, p := range partialsWritten {
		partialsChunks = append(partialsChunks, &chunk{
			start:       p.StartBlock,
			end:         p.ExclusiveEndBlock,
			// TODO(abourget): tant qu'à computer ça à chaque fois,
			// autant le computer au moment où on devrait s'en servir,
			// ce qui veut dire qu'on le ferait à un seul endroit,
			// qu'on pourrait utiliser des `block.Range` normaux à la
			// place de cet objet.
			//
			// Le seul risque: si la config de `storeSaveInterval` est
			// différent entre l'orchestrateur et le backprocessing
			// node, on pourrait considérer certain stores comme étant
			// temporaire et les effacer. C'est plus ou moins un
			// problème maintenant qu'on query le ObjectStore pour
			// savoir ce qu'il y a effectivement.
			tempPartial: p.ExclusiveEndBlock%job.moduleSaveInterval != 0,
		})
	}

	if err = s.Callback(ctx, job, partialsChunks); err != nil {
		return fmt.Errorf("calling back scheduler: %w", err)
	}
	return nil

}
