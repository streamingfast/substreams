package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
)

type Backprocessor struct {
	plan             *work.Plan
	scheduler        *Scheduler
	squasher         *MultiSquasher
	workerPool       work.WorkerPool
	execOutputReader *LinearExecOutputReader
}

func BuildBackProcessor(
	ctx context.Context,
	runtimeConfig config.RuntimeConfig,
	upToBlock uint64,
	outputGraph *outputmodules.Graph,
	execoutStorage *execout.Configs,
	respFunc func(resp *pbsubstreams.Response) error,
	storeConfigs store.ConfigMap,
	upstreamRequestModules *pbsubstreams.Modules,
) (*Backprocessor, error) {
	modulesStateMap, err := storage.BuildModuleStorageStateMap(ctx, storeConfigs, runtimeConfig.StoreSnapshotsSaveInterval, execoutStorage, runtimeConfig.ExecOutputSaveInterval, upToBlock)
	if err != nil {
		return nil, fmt.Errorf("build storage map: %w", err)
	}

	plan, err := work.BuildNewPlan(modulesStateMap, runtimeConfig.SubrequestsSplitSize, upToBlock, outputGraph)
	if err != nil {
		return nil, fmt.Errorf("build work plan: %w", err)
	}

	if err := plan.SendInitialProgressMessages(respFunc); err != nil {
		return nil, fmt.Errorf("send initial progress: %w", err)
	}

	scheduler := NewScheduler(plan, respFunc, upstreamRequestModules)
	if err != nil {
		return nil, err
	}

	squasher, err := NewMultiSquasher(ctx, runtimeConfig, plan.ModulesStateMap, storeConfigs, upToBlock, scheduler.OnStoreCompletedUntilBlock)
	if err != nil {
		return nil, err
	}

	scheduler.OnStoreJobTerminated = squasher.Squash

	runnerPool := work.NewWorkerPool(ctx, runtimeConfig.ParallelSubrequests, runtimeConfig.WorkerFactory)

	return &Backprocessor{
		plan:       plan,
		scheduler:  scheduler,
		squasher:   squasher,
		workerPool: runnerPool,
	}, nil
}

func (b *Backprocessor) Run(ctx context.Context) (storeMap store.Map, err error) {
	if b.execOutputReader != nil {
		b.execOutputReader.Launch(ctx)
	}
	b.squasher.Launch(ctx)

	if err := b.scheduler.Schedule(ctx, b.workerPool); err != nil {
		return nil, fmt.Errorf("scheduler run: %w", err)
	}

	storeMap, err = b.squasher.Wait(ctx)
	if err != nil {
		return nil, err
	}

	if b.execOutputReader != nil {
		select {
		case <-b.execOutputReader.Terminated():
		case <-ctx.Done():
		}
	}
	return storeMap, nil
}

func (b *Backprocessor) SetOutputReader(execOutputReader *LinearExecOutputReader) {
	b.execOutputReader = execOutputReader
}
