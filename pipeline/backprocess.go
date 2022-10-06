package pipeline

import (
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

func (p *Pipeline) backProcessStores(
	workerPool *orchestrator.WorkerPool,
	storeModules []*pbsubstreams.Module,
) (out map[string]store.Store, err error) {
	p.reqCtx.StartSpan("back_processing", p.tracer)
	defer p.reqCtx.EndSpan(err)

	logger := p.reqCtx.logger.Named("back_process")
	logger.Info("synchronizing stores")

	var storageState *orchestrator.StorageState
	if storageState, err = orchestrator.FetchStorageState(p.reqCtx, p.storeMap); err != nil {
		err = fmt.Errorf("fetching stores states: %w", err)
		return nil, err
	}

	logger.Info("storage state found")
	workPlan := orchestrator.WorkPlan{}
	for _, mod := range storeModules {
		snapshot, ok := storageState.Snapshots[mod.Name]
		if !ok {
			err = fmt.Errorf("fatal: storage state not reported for module name %q", mod.Name)
			return nil, err
		}
		workPlan[mod.Name] = orchestrator.SplitWork(mod.Name, p.storeFactory.saveInterval, mod.InitialBlock, p.reqCtx.StartBlockNum(), snapshot)
	}

	logger.Info("work plan ready", zap.Stringer("work_plan", workPlan))

	progressMessages := workPlan.ProgressMessages()
	if err = p.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		err = fmt.Errorf("sending progress: %w", err)
		return nil, err
	}

	upToBlock := p.reqCtx.StartBlockNum()

	var jobsPlanner *orchestrator.JobsPlanner
	if jobsPlanner, err = orchestrator.NewJobsPlanner(p.reqCtx, workPlan, uint64(p.subrequestSplitSize), p.graph); err != nil {
		err = fmt.Errorf("creating strategy: %w", err)
		return nil, err
	}

	logger.Debug("launching squasher")

	var squasher *orchestrator.Squasher
	if squasher, err = orchestrator.NewSquasher(p.reqCtx, workPlan, p.storeMap, upToBlock, p.storeFactory.saveInterval, jobsPlanner); err != nil {
		err = fmt.Errorf("initializing squasher: %w", err)
		return nil, err
	}

	if err = workPlan.SquashPartialsPresent(squasher); err != nil {
		return nil, err
	}

	var scheduler *orchestrator.Scheduler
	if scheduler, err = orchestrator.NewScheduler(p.reqCtx, jobsPlanner.AvailableJobs, squasher, workerPool, p.respFunc); err != nil {
		err = fmt.Errorf("initializing scheduler: %w", err)
		return nil, err
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
	squasher.Shutdown(nil)

	if out, err = squasher.ValidateStoresReady(); err != nil {
		err = fmt.Errorf("squasher incomplete: %w", err)
		return nil, err
	}
	return out, nil
}
