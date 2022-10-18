package pipeline

import (
	"fmt"

	"github.com/streamingfast/substreams/tracing"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

func (p *Pipeline) backProcessStores(
	workerPool *orchestrator.WorkerPool,
	storeConfigs []*store.Config,
) (out store.Map, err error) {
	_, span := p.tracer.Start(p.reqCtx.Context, "back_processing")
	defer tracing.EndSpan(span, tracing.WithEndErr(&err))

	logger := p.reqCtx.logger.Named("back_process")
	logger.Info("synchronizing stores")

	storageState, err := orchestrator.FetchStorageState(p.reqCtx, storeConfigs)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	logger.Info("storage state found")

	upToBlock := p.reqCtx.EffectiveStartBlockNum()

	workPlan := orchestrator.WorkPlan{}

	for _, config := range storeConfigs {
		name := config.Name()
		snapshot, ok := storageState.Snapshots[name]
		if !ok {
			return nil, fmt.Errorf("fatal: storage state not reported for module name %q", name)
		}
		workPlan[name] = orchestrator.SplitWork(name, p.storeConfig.SaveInterval, config.ModuleInitialBlock(), upToBlock, snapshot)
	}
	logger.Info("work plan ready", zap.Stringer("work_plan", workPlan))

	progressMessages := workPlan.ProgressMessages()
	if err := p.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return nil, fmt.Errorf("sending progress: %w", err)
	}

	jobsPlanner, err := orchestrator.NewJobsPlanner(p.reqCtx, workPlan, uint64(p.subrequestSplitSize), p.graph)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	logger.Debug("launching squasher")

	squasher, err := orchestrator.NewSquasher(p.reqCtx, workPlan, storeConfigs, upToBlock, p.storeConfig.SaveInterval, jobsPlanner)
	if err != nil {
		return nil, fmt.Errorf("initializing squasher: %w", err)
	}

	if err = workPlan.SquashPartialsPresent(squasher); err != nil {
		return nil, err
	}

	scheduler, err := orchestrator.NewScheduler(p.reqCtx, jobsPlanner.AvailableJobs, squasher, workerPool, p.respFunc)
	if err != nil {
		return nil, fmt.Errorf("initializing scheduler: %w", err)
	}

	result := make(chan error)

	logger.Debug("launching scheduler")

	go scheduler.Launch(p.reqCtx, p.reqCtx.Request().Modules, result)

	jobCount := jobsPlanner.JobCount()
	for resultCount := 0; resultCount < jobCount; {
		select {
		case <-p.reqCtx.Done():
			if err = p.reqCtx.Err(); err != nil {
				return nil, err
			}
			logger.Info("job canceled")
			return nil, nil
		case err = <-result:
			resultCount++
			if err != nil {
				err = fmt.Errorf("from worker: %w", err)
				return nil, err
			}
			logger.Debug("received result", zap.Int("result_count", resultCount), zap.Int("job_count", jobCount))
		}
	}

	logger.Info("all jobs completed, waiting for squasher to finish")
	if err := squasher.WaitUntilCompleted(p.reqCtx); err != nil {
		return nil, fmt.Errorf("squasher failed: %w", err)
	}

	if out, err = squasher.ValidateStoresReady(); err != nil {
		return nil, fmt.Errorf("squasher incomplete: %w", err)
	}

	return
}
