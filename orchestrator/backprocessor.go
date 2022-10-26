package orchestrator

import (
	"context"
	"fmt"
	
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/work"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/store"
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
func (b *Backprocessor) Run(ctx context.Context, plan *work.Plan, scheduler *Scheduler, squasher *MultiSquasher, pool work.JobRunnerPool) (store.Map, error) {
	// workPlan should hold all the jobs, dependencies
	// and it could be a changing plan, reshufflable,
	// This contains all what the jobsPlanner had
	// This calls `splitWork`, with save Interval, moduleInitialBlock, snapshots

	err := b.init(plan)
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	// parallelDownloader := NewLinearExecOutputReader()
	// go parallelDownloader.Launch()
	squasher.Launch(ctx)

	if err := scheduler.Schedule(ctx, pool); err != nil {
		return nil, fmt.Errorf("scheduler run: %w", err)
	}

	finalStoreMap, err := squasher.Wait(ctx)
	if err != nil {
		return nil, err
	}

	// if err := parallelDownloader.Wait(); err != nil {
	// 	return nil, err
	// }

	return finalStoreMap, nil
}

func (b *Backprocessor) init(workPlan *work.Plan) (err error) {
	progressMessages := workPlan.InitialProgressMessages()
	if err := b.respFunc(substreams.NewModulesProgressResponse(progressMessages)); err != nil {
		return err
	}
	return nil
}
