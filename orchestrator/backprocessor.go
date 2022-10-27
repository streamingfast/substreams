package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/reqctx"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

type Backprocessor struct {
	runtimeConfig                config.RuntimeConfig
	storeConfigMap               store.ConfigMap
	upToBlock                    uint64 // We stop at this block exclusively. This is an irreversible block.
	graph                        *manifest.ModuleGraph
	upstreamRequestModules       *pbsubstreams.Modules
	upstreamRequestOutputModules []string // TODO(abourget): we'll need to distinguish here if upstream wants parallel download for those, so perhaps another map. This will mean we'll probably want to adjust the gRPC Request/Response models, to have flags instead of list of output modules. See the GitHub issues with details and an example. # ? :)
	respFunc                     func(resp *pbsubstreams.Response) error
}

func New(
	runtimeConfig config.RuntimeConfig,
	upToBlock uint64,
	graph *manifest.ModuleGraph,
	respFunc func(resp *pbsubstreams.Response) error,
	storeConfigs store.ConfigMap,
	upstreamRequestModules *pbsubstreams.Modules,
) *Backprocessor {
	return &Backprocessor{
		runtimeConfig:          runtimeConfig,
		upToBlock:              upToBlock,
		graph:                  graph,
		respFunc:               respFunc,
		storeConfigMap:         storeConfigs,
		upstreamRequestModules: upstreamRequestModules,
	}
}

// TODO(abourget): WARN: this function should NOT GROW in functionality, or abstraction levels.
func (b *Backprocessor) Run(ctx context.Context) (store.Map, error) {
	logger := reqctx.Logger(ctx)
	// workPlan should hold all the jobs, dependencies
	// and it could be a changing plan, reshufflable,
	// This contains all what the jobsPlanner had
	// This calls `splitWork`, with save Interval, moduleInitialBlock, snapshots
	workPlan, err := b.planWork(ctx)
	if err != nil {
		return nil, err
	}

	scheduler, err := NewScheduler(ctx, b.runtimeConfig, workPlan, b.graph, b.respFunc, logger, b.upstreamRequestModules)
	if err != nil {
		return nil, err
	}

	multiSquasher, err := NewMultiSquasher(ctx, b.runtimeConfig, workPlan, b.storeConfigMap, b.upToBlock, scheduler.OnStoreCompletedUntilBlock)
	if err != nil {
		return nil, err
	}

	scheduler.OnStoreJobTerminated = multiSquasher.Squash

	// parallelDownloader := NewLinearExecOutputReader()
	// go parallelDownloader.Launch()
	multiSquasher.Launch(ctx)

	if err := scheduler.Run(ctx); err != nil {
		return nil, fmt.Errorf("scheduler run: %w", err)
	}

	finalStoreMap, err := multiSquasher.Wait(ctx)
	if err != nil {
		return nil, err
	}

	// if err := parallelDownloader.Wait(); err != nil {
	// 	return nil, err
	// }

	return finalStoreMap, nil
}

func (b *Backprocessor) planWork(ctx context.Context) (out *WorkPlan, err error) {
	storageState, err := fetchStorageState(ctx, b.storeConfigMap)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	workPlan, err := b.buildWorkPlan(ctx, storageState)
	if err != nil {
		return nil, fmt.Errorf("build work plan: %w", err)
	}

	if err := b.sendWorkPlanProgress(workPlan); err != nil {
		return nil, fmt.Errorf("sending work plan progress: %w", err)
	}

	return workPlan, nil
}

func (b *Backprocessor) buildWorkPlan(ctx context.Context, storageState *StorageState) (out *WorkPlan, err error) {
	logger := reqctx.Logger(ctx)
	out = &WorkPlan{
		workUnitsMap: map[string]*WorkUnits{}, // per module
	}
	for _, config := range b.storeConfigMap {
		name := config.Name()
		snapshot, ok := storageState.Snapshots[name]
		if !ok {
			return nil, fmt.Errorf("fatal: storage state not reported for module name %q", name)
		}

		wu := &WorkUnits{modName: name}
		if err := wu.init(b.runtimeConfig.StoreSnapshotsSaveInterval, config.ModuleInitialBlock(), b.upToBlock, snapshot); err != nil {
			return nil, fmt.Errorf("init worker unit %q: %w", name, err)
		}

		out.workUnitsMap[name] = wu
	}
	logger.Info("work plan ready", zap.Stringer("work_plan", out))
	return
}

func (b *Backprocessor) sendWorkPlanProgress(workPlan *WorkPlan) (err error) {
	progressMessages := workPlan.ProgressMessages()
	if err := b.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return err
	}
	return nil
}
