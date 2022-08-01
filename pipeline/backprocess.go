package pipeline

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/state"
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
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		zlog.Debug("back processing canceling ctx", zap.Error(ctx.Err()))
	}()

	zlog.Info("synchronizing stores")

	storageState, err := orchestrator.FetchStorageState(ctx, initialStoreMap)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	zlog.Info("storage state found", zap.Stringer("storage state", storageState))

	workPlan := orchestrator.WorkPlan{}
	for _, mod := range p.storeModules {
		snapshot, ok := storageState.Snapshots[mod.Name]
		if !ok {
			return nil, fmt.Errorf("fatal: storage state not reported for module name %q", mod.Name)
		}
		workPlan[mod.Name] = orchestrator.SplitWork(mod.Name, p.storeSaveInterval, mod.InitialBlock, uint64(p.request.StartBlockNum), snapshot)
	}

	zlog.Info("work plan ready", zap.Stringer("work_plan", workPlan))

	progressMessages := workPlan.ProgressMessages()
	if err := p.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return nil, fmt.Errorf("sending progress: %w", err)
	}

	upToBlock := uint64(p.request.StartBlockNum)

	jobsPlanner, err := orchestrator.NewJobsPlanner(ctx, workPlan, uint64(p.subrequestSplitSize), initialStoreMap, p.graph)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	zlog.Debug("launching squasher")

	squasher, err := orchestrator.NewSquasher(ctx, workPlan, initialStoreMap, upToBlock, jobsPlanner)
	if err != nil {
		return nil, fmt.Errorf("initializing squasher: %w", err)
	}

	err = workPlan.SquashPartialsPresent(squasher)
	if err != nil {
		return nil, err
	}

	scheduler, err := orchestrator.NewScheduler(ctx, jobsPlanner.AvailableJobs, squasher, workerPool, p.respFunc)
	if err != nil {
		return nil, fmt.Errorf("initializing scheduler: %w", err)
	}

	result := make(chan error)

	zlog.Debug("launching scheduler")

	go scheduler.Launch(ctx, p.request.Modules, result)

	jobCount := jobsPlanner.JobCount()
	for resultCount := 0; resultCount < jobCount; {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-result:
			resultCount++
			if err != nil {
				return nil, fmt.Errorf("from worker: %w", err)
			}
			zlog.Debug("received result", zap.Int("result_count", resultCount), zap.Int("job_count", jobCount), zap.Error(err))
		}
	}

	zlog.Info("all jobs completed, waiting for squasher to finish")
	squasher.Shutdown(nil)

	newStores, err := squasher.ValidateStoresReady()
	if err != nil {
		return nil, fmt.Errorf("squasher incomplete: %w", err)
	}

	return newStores, nil
}
