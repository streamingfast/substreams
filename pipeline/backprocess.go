package pipeline

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/state"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

func (p *Pipeline) backProcessStores(
	ctx context.Context,
	workerPool *orchestrator.WorkerPool,
	initialStoreMap map[string]*state.Store,
) (
	map[string]*state.Store,
	error,
) {
	ctx, span := p.tracer.Start(p.context, "back_processing")
	defer span.End()

	logger := p.logger.Named("back_process")
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		logger.Debug("back processing canceling ctx", zap.Error(ctx.Err()))
	}()

	logger.Info("synchronizing stores")

	storageState, err := orchestrator.FetchStorageState(ctx, initialStoreMap)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	logger.Info("storage state found")

	workPlan := orchestrator.WorkPlan{}
	for _, mod := range p.storeModules {
		snapshot, ok := storageState.Snapshots[mod.Name]
		if !ok {
			err := fmt.Errorf("fatal: storage state not reported for module name %q", mod.Name)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		workPlan[mod.Name] = orchestrator.SplitWork(mod.Name, p.storeSaveInterval, mod.InitialBlock, uint64(p.request.StartBlockNum), snapshot)
	}

	logger.Info("work plan ready", zap.Stringer("work_plan", workPlan))

	progressMessages := workPlan.ProgressMessages()
	if err := p.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("sending progress: %w", err)
	}

	upToBlock := uint64(p.request.StartBlockNum)

	jobsPlanner, err := orchestrator.NewJobsPlanner(ctx, workPlan, uint64(p.subrequestSplitSize), initialStoreMap, p.graph)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	logger.Debug("launching squasher")

	squasher, err := orchestrator.NewSquasher(ctx, workPlan, initialStoreMap, upToBlock, jobsPlanner)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("initializing squasher: %w", err)
	}

	err = workPlan.SquashPartialsPresent(squasher)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	scheduler, err := orchestrator.NewScheduler(ctx, jobsPlanner.AvailableJobs, squasher, workerPool, p.respFunc)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("initializing scheduler: %w", err)
	}

	result := make(chan error)

	logger.Debug("launching scheduler")

	go scheduler.Launch(ctx, p.request.Modules, result)

	jobCount := jobsPlanner.JobCount()
	for resultCount := 0; resultCount < jobCount; {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}
			span.SetStatus(codes.Ok, "canceled")
			return nil, nil
		case err := <-result:
			resultCount++
			if err != nil {
				span.SetStatus(codes.Error, err.Error())
				return nil, fmt.Errorf("from worker: %w", err)
			}
			logger.Debug("received result", zap.Int("result_count", resultCount), zap.Int("job_count", jobCount), zap.Error(err))
		}
	}

	logger.Info("all jobs completed, waiting for squasher to finish")
	squasher.Shutdown(nil)

	newStores, err := squasher.ValidateStoresReady()
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("squasher incomplete: %w", err)
	}
	span.SetStatus(codes.Ok, "completed")
	return newStores, nil
}
