package pipeline

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/orchestrator"
	"github.com/streamingfast/substreams/orchestrator/worker"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

func (p *Pipeline) backprocessStores(
	ctx context.Context,
	workerPool *worker.Pool,
	respFunc substreams.ResponseFunc,
) (
	map[string]*state.Store,
	error,
) {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	zlog.Info("synchronizing stores")

	requestPool := orchestrator.NewRequestPool()

	storageState, err := orchestrator.FetchStorageState(ctx, p.storeMap)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	progressMessages := storageState.ProgressMessages()
	if err := respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return nil, fmt.Errorf("sending progress: %w", err)
	}

	upToBlock := uint64(p.request.StartBlockNum)

	squasher, err := orchestrator.NewSquasher(ctx, storageState, p.storeMap, p.storesSaveInterval, upToBlock, orchestrator.WithNotifier(requestPool))
	if err != nil {
		return nil, fmt.Errorf("initializing squasher: %w", err)
	}

	strategy, err := orchestrator.NewOrderedStrategy(ctx, storageState, p.request, p.storeMap, p.graph, requestPool, upToBlock, p.blockRangeSizeSubRequests, p.maxStoreSyncRangeSize)
	if err != nil {
		return nil, fmt.Errorf("creating strategy: %w", err)
	}

	scheduler, err := orchestrator.NewScheduler(ctx, strategy, squasher, workerPool, respFunc, p.blockRangeSizeSubRequests)
	if err != nil {
		return nil, fmt.Errorf("initializing scheduler: %w", err)
	}

	result := make(chan error)

	scheduler.Launch(ctx, result)

	requestCount := strategy.RequestCount()
	resultCount := 0
done:
	for {
		select {
		case <-ctx.Done():
			return nil, context.Canceled // FIXME: If we exit here without killing the go func() above, this will clog the `result` chan
		case err := <-result:
			resultCount++
			if err != nil {
				return nil, fmt.Errorf("from worker: %w", err)
			}
			zlog.Debug("received result", zap.Int("result_count", resultCount), zap.Int("request_count", requestCount), zap.Error(err))
			if resultCount == requestCount {
				break done
			}
		}
	}

	zlog.Info("store sync completed")

	newStores, err := squasher.StoresReady()
	if err != nil {
		return nil, fmt.Errorf("squasher incomplete: %w", err)
	}

	return newStores, nil
}
