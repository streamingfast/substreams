package orchestrator

import (
	"context"

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
) *Backprocessor {
	return &Backprocessor{
		ctx:           ctx,
		runtimeConfig: runtimeConfig,
		upToBlock:     upToBlock,
		log:           logger,
		graph:         graph,
		respFunc:      respFunc,
		storeConfigs:  storeConfigs,
	}
}

// TODO(abourget): WARN: this function should NOT GROW in functionality, or abstraction levels.
func (b *Backprocessor) Run() (store.Map, error) {
	// workPlan should hold all the jobs, dependencies
	// and it could be a changing plan, reshufflable,
	// This contains all what the jobsPlanner had
	// This calls `SplitWork`, with save Interval, moduleInitialBlock, snapshots
	workPlan, err := b.buildWorkPlan()
	if err != nil {
		return nil, err
	}

	scheduler, err := NewScheduler(b.ctx, workPlan, b.runtimeConfig)
	if err != nil {
		return nil, err
	}

	multiSquasher, err := NewMultiSquasher(b.ctx, workPlan, b.storeConfigs, b.upToBlock, b.runtimeConfig)
	if err != nil {
		return nil, err
	}

	multiSquasher.OnStoreCompletedUntilBlock = scheduler.OnStoreCompletedUntilBlock
	scheduler.OnJobTerminated = multiSquasher.Squash

	// parallelDownloader := NewLinearExecOutputReader()
	// go parallelDownloader.Launch()
	go multiSquasher.Launch()
	go scheduler.Launch()

	finalStoreMap, err := multiSquasher.Wait()
	if err != nil {
		return nil, err
	}

	// if err := parallelDownloader.Wait(); err != nil {
	// 	return nil, err
	// }

	return finalStoreMap, nil
}
