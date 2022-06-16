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

	jobPool := orchestrator.NewJobPool()

	storageState, err := orchestrator.FetchStorageState(ctx, initialStoreMap)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	workPlan := orchestrator.WorkPlan{}
	for _, mod := range p.storeModules {
		workPlan[mod.Name] = orchestrator.SplitWork(mod.Name, p.storeSaveInterval, mod.InitialBlock, uint64(p.request.StartBlockNum), storageState.Snapshots[mod.Name])
	}

	progressMessages := workPlan.ProgressMessages()
	if err := p.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return nil, fmt.Errorf("sending progress: %w", err)
	}

	upToBlock := uint64(p.request.StartBlockNum)

	strategy, err := orchestrator.NewOrderedStrategy(ctx, workPlan, uint64(p.subrequestSplitSize), initialStoreMap, p.graph, jobPool)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	squasher, err := orchestrator.NewSquasher(ctx, workPlan, initialStoreMap, upToBlock, jobPool)
	if err != nil {
		return nil, fmt.Errorf("initializing squasher: %w", err)
	}

	err = workPlan.SquashPartialsPresent(ctx, squasher)
	if err != nil {
		return nil, err
	}

	scheduler, err := orchestrator.NewScheduler(ctx, strategy, squasher, workerPool, p.respFunc)
	if err != nil {
		return nil, fmt.Errorf("initializing scheduler: %w", err)
	}

	result := make(chan error)

	go scheduler.Launch(ctx, result)

	requestCount := jobPool.Count() // Is this expected to be the TOTAL number of requests we've seen?
	for resultCount := 0; resultCount < requestCount; {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-result:
			resultCount++
			if err != nil {
				return nil, fmt.Errorf("from worker: %w", err)
			}
			zlog.Debug("received result", zap.Int("result_count", resultCount), zap.Int("request_count", requestCount), zap.Error(err))
		}
	}

	zlog.Info("store sync completed")

	newStores, err := squasher.ValidateStoresReady()
	if err != nil {
		return nil, fmt.Errorf("squasher incomplete: %w", err)
	}

	return newStores, nil
}
