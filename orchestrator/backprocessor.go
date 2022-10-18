package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

type Backprocessor struct {
	ctx           context.Context
	runtimeConfig config.RuntimeConfig
	storeConfigs  store.ConfigMap
	log           *zap.Logger

	upToBlock uint64 // We stop at this block exclusively. This is an irreversible block.

	graph                        *manifest.ModuleGraph
	upstreamRequestModules       *pbsubstreams.Modules
	upstreamRequestOutputModules []string // TODO(abourget): we'll need to distinguish here if upstream wants parallel download for those, so perhaps another map. This will mean we'll probably want to adjust the gRPC Request/Response models, to have flags instead of list of output modules. See the GitHub issues with details and an example. # ? :)

	respFunc func(resp *pbsubstreams.Response) error

	// respFunc
	// storeConfigs

	// BuildWorkPlan()
	// workPlan
	// workerPool
}

func New(
	ctx context.Context,
	runtimeConfig config.RuntimeConfig,
	logger *zap.Logger,
	upToBlock uint64,
	graph *manifest.ModuleGraph,
	respFunc func(resp *pbsubstreams.Response) error,
	storeConfigs store.ConfigMap,
	upstreamRequestModules *pbsubstreams.Modules,
) *Backprocessor {
	return &Backprocessor{
		ctx:                    ctx,
		runtimeConfig:          runtimeConfig,
		upToBlock:              upToBlock,
		log:                    logger,
		graph:                  graph,
		respFunc:               respFunc,
		storeConfigs:           storeConfigs,
		upstreamRequestModules: upstreamRequestModules,
	}
}

// TODO(abourget): WARN: this function should NOT GROW in functionality, or abstraction levels.
func (b *Backprocessor) Run() (store.Map, error) {
	// workPlan should hold all the jobs, dependencies
	// and it could be a changing plan, reshufflable,
	// This contains all what the jobsPlanner had
	// This calls `SplitWork`, with save Interval, moduleInitialBlock, snapshots
	workPlan, err := b.planWork()
	if err != nil {
		return nil, err
	}

	scheduler, err := NewScheduler(b.ctx, b.runtimeConfig, workPlan, b.graph, b.respFunc, b.log, b.upstreamRequestModules)
	if err != nil {
		return nil, err
	}

	multiSquasher, err := NewMultiSquasher(b.ctx, b.runtimeConfig, workPlan, b.storeConfigs, b.upToBlock, scheduler.OnStoreCompletedUntilBlock)
	if err != nil {
		return nil, err
	}

	scheduler.OnStoreJobTerminated = multiSquasher.Squash

	// parallelDownloader := NewLinearExecOutputReader()
	// go parallelDownloader.Launch()
	multiSquasher.Launch(b.ctx)

	if err := scheduler.Run(b.ctx, b.upstreamRequestModules); err != nil {
		return nil, fmt.Errorf("scheduler: %w", err)
	}

	finalStoreMap, err := multiSquasher.Wait(b.ctx)
	if err != nil {
		return nil, err
	}

	// if err := parallelDownloader.Wait(); err != nil {
	// 	return nil, err
	// }

	return finalStoreMap, nil
}

func (b *Backprocessor) planWork() (out *WorkPlan, err error) {
	storageState, err := fetchStorageState(b.ctx, b.storeConfigs)
	if err != nil {
		return nil, fmt.Errorf("fetching stores states: %w", err)
	}

	workPlan, err := b.buildWorkPlan(storageState)
	if err != nil {
		return nil, fmt.Errorf("build work plan: %w", err)
	}

	if err := b.sendWorkPlanProgress(workPlan); err != nil {
		return nil, fmt.Errorf("sending work plan progress: %w", err)
	}

	return workPlan, nil
}

func (b *Backprocessor) buildWorkPlan(storageState *StorageState) (out *WorkPlan, err error) {
	out = &WorkPlan{
		workUnitsMap: map[string]*WorkUnits{}, // per module
	}
	for _, config := range b.storeConfigs {
		name := config.Name()
		snapshot, ok := storageState.Snapshots[name]
		if !ok {
			return nil, fmt.Errorf("fatal: storage state not reported for module name %q", name)
		}
		// TODO(abourget): Pass in the `SaveInterval` in some ways
		out.workUnitsMap[name] = SplitWork(name, b.runtimeConfig.StoreSnapshotsSaveInterval, config.ModuleInitialBlock(), b.upToBlock, snapshot)
	}
	b.log.Info("work plan ready", zap.Stringer("work_plan", out))

	return
}

func (b *Backprocessor) sendWorkPlanProgress(workPlan *WorkPlan) (err error) {
	progressMessages := workPlan.ProgressMessages()
	if err := b.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return err
	}
	return nil
}
