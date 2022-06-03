package pipeline

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

func (p *Pipeline) backprocessStores(
	ctx context.Context,
	workerPool *orchestrator.WorkerPool,
	respFunc substreams.ResponseFunc,
) (
	map[string]*state.Store,
	error,
) {

	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		zlog.Debug("backprocessing canceling ctx", zap.Error(ctx.Err()))
	}()

	zlog.Info("synchronizing stores")

	requestPool := orchestrator.NewRequestPool()

	storageState, err := orchestrator.FetchStorageState(ctx, p.storeMap)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	splitWorks := orchestrator.SplitWorkModules{}
	for _, mod := range p.storeModules {
		splitWorks[mod.Name] = orchestrator.SplitSomeWork(mod.Name, p.storesSaveInterval, uint64(p.blockRangeSizeSubRequests), mod.InitialBlock, storageState.LastBlock(mod.Name), uint64(p.request.StartBlockNum))
	}

	progressMessages := splitWorks.ProgressMessages()
	if err := respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return nil, fmt.Errorf("sending progress: %w", err)
	}

	upToBlock := uint64(p.request.StartBlockNum)

	strategy, err := orchestrator.NewOrderedStrategy(ctx, splitWorks, p.request, p.storeMap, p.graph, requestPool)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	squasher, err := orchestrator.NewSquasher(ctx, splitWorks, p.storeMap, upToBlock, orchestrator.WithNotifier(requestPool))
	if err != nil {
		return nil, fmt.Errorf("initializing squasher: %w", err)
	}

	scheduler, err := orchestrator.NewScheduler(ctx, strategy, squasher, workerPool, respFunc, p.blockRangeSizeSubRequests)
	if err != nil {
		return nil, fmt.Errorf("initializing scheduler: %w", err)
	}

	result := make(chan error)

	schedulerErr := scheduler.Launch(ctx, result)

	requestCount := requestPool.Count() // Is this expected to be the TOTAL number of requests we've seen?
	for resultCount := 0; resultCount < requestCount; {
		select {
		case <-ctx.Done():
			return nil, ctx.Err() // FIXME: If we exit here without killing the go func() above, this will clog the `result` chan
		case err := <-schedulerErr:
			if err == nil {
				continue
			}
			return nil, fmt.Errorf("scheduler: %w", err)
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
