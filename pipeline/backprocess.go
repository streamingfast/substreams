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

	requestPool := orchestrator.NewRequestPool()

	initialStoreMap, err := p.buildStoreMap()
	if err != nil {
		return nil, fmt.Errorf("build initial store map: %w", err)
	}

	storageState, err := orchestrator.FetchStorageState(ctx, initialStoreMap)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	splitWorks := orchestrator.WorkPlan{}
	for _, mod := range p.storeModules {
		splitWorks[mod.Name] = orchestrator.SplitWork(mod.Name, p.storeSaveInterval, mod.InitialBlock, uint64(p.request.StartBlockNum), storageState.Snapshots[mod.Name])
	}

	progressMessages := splitWorks.ProgressMessages()
	if err := p.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return nil, fmt.Errorf("sending progress: %w", err)
	}

	upToBlock := uint64(p.request.StartBlockNum)

	strategy, err := orchestrator.NewOrderedStrategy(ctx, splitWorks, uint64(p.subreqSplitSize), initialStoreMap, p.graph, requestPool)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	squasher, err := orchestrator.NewSquasher(ctx, splitWorks, initialStoreMap, upToBlock, requestPool)
	if err != nil {
		return nil, fmt.Errorf("initializing squasher: %w", err)
	}

	scheduler, err := orchestrator.NewScheduler(ctx, strategy, squasher, workerPool, p.respFunc)
	if err != nil {
		return nil, fmt.Errorf("initializing scheduler: %w", err)
	}

	result := make(chan error)

	go scheduler.Launch(ctx, result)

	requestCount := requestPool.Count() // Is this expected to be the TOTAL number of requests we've seen?
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

	newStores, err := squasher.StoresReady()
	if err != nil {
		return nil, fmt.Errorf("squasher incomplete: %w", err)
	}

	return newStores, nil
}
