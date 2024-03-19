package orchestrator

import (
	"context"
	"fmt"
	"os"

	"github.com/streamingfast/substreams"
	orchestratorExecout "github.com/streamingfast/substreams/orchestrator/execout"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/scheduler"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
)

type ParallelProcessor struct {
	scheduler *scheduler.Scheduler
	reqPlan   *plan.RequestPlan
}

// BuildParallelProcessor is only called on tier1
func BuildParallelProcessor(
	ctx context.Context,
	reqPlan *plan.RequestPlan,
	runtimeConfig config.RuntimeConfig,
	maxParallelJobs int,
	outputGraph *outputmodules.Graph,
	execoutStorage *execout.Configs,
	respFunc func(resp substreams.ResponseFromAnyTier) error,
	storeConfigs store.ConfigMap,
) (*ParallelProcessor, error) {

	stream := response.New(respFunc)
	sched := scheduler.New(ctx, stream)

	stages := stage.NewStages(ctx, outputGraph, reqPlan, storeConfigs)
	sched.Stages = stages

	// OPTIMIZATION: We should fetch the ExecOut files too, and see if they
	// cover some of the ranges that we're after.
	// We don't need to plan work for ranges where we have ExecOut
	// already.
	// BUT we'll need to have stores to be able to schedule work after
	// so there's a mix of FullKV stores and ExecOut files we need
	// to check.  We can push the `segmentCompleted` based on the
	// execout files.

	// The previous code did what? Just assumed there was ExecOut files
	// prior to the latest Complete snapshot?

	// FIXME: Is the state map the final reference for the progress we've made?
	// Shouldn't that be processed by the scheduler a little bit?
	// What if we have discovered a bunch of ExecOut files and the scheduler
	// would decide not to use the very first stores as a sign of what is complete?
	// Well, perhaps those wouldn't hurt, because here we're _sure_ they're
	// done and the Scheduler could send Progress messages when the above decision
	// is taken.

	// FIXME: Are all the progress messages properly sent? When we skip some stores and mark them complete,
	// for whatever reason,

	if reqPlan.ReadExecOut != nil {
		execOutSegmenter := reqPlan.WriteOutSegmenter()
		// note: since we are *NOT* in a sub-request and are setting up output module is a map
		requestedModule := outputGraph.OutputModule()
		if requestedModule.GetKindStore() != nil {
			panic("logic error: should not get a store as outputModule on tier 1")
		}

		walker := execoutStorage.NewFileWalker(requestedModule.Name, execOutSegmenter)

		sched.ExecOutWalker = orchestratorExecout.NewWalker(
			ctx,
			requestedModule,
			walker,
			reqPlan.ReadExecOut,
			stream,
		)
	}

	// we may be here only for mapper, without stores
	if reqPlan.BuildStores != nil {
		err := stages.FetchStoresState(
			ctx,
			reqPlan.StoresSegmenter(),
			storeConfigs,
			execoutStorage,
		)
		if err != nil {
			return nil, fmt.Errorf("fetch stores storage state: %w", err)
		}
	} else {
		err := stages.FetchStoresState(
			ctx,
			reqPlan.WriteOutSegmenter(),
			storeConfigs,
			execoutStorage,
		)
		if err != nil {
			return nil, fmt.Errorf("fetch stores storage state: %w", err)
		}

	}

	if os.Getenv("SUBSTREAMS_DEBUG_SCHEDULER_STATE") == "true" {
		fmt.Println("Initial state:")
		fmt.Print(stages.StatesString())
	}

	// OPTIMIZE(abourget): take all of the ExecOut files that exist
	//  and use that to PUSH back what the Stages need to do.
	//  So the first Segment to process will not necessarily be
	//  segment == 0.  We'll need the segment JUST prior to be
	//  processed though, because we need to continue working on
	//  the future segments.  This interplays with the segment
	//  just before.
	//  -
	//  In other words, if we can stream out the ExecOut directly, we don't need
	//  to dispatch work to process them at all.  But we'll need
	//  to have stores ready to continue segments work.
	//  SO: we can move forward the processing pipeline, provided
	//  all of the stages can be continued forward after the
	//  last ExecOut segment: that we have complete stores for the
	//  segment where ExecOut finishes.
	//  -
	//  This is an optimization and is not solved herein.

	workerPool := work.NewWorkerPool(ctx, maxParallelJobs, runtimeConfig.WorkerFactory)
	sched.WorkerPool = workerPool

	return &ParallelProcessor{
		scheduler: sched,
		reqPlan:   reqPlan,
	}, nil
}

func (b *ParallelProcessor) Stages() *stage.Stages {
	return b.scheduler.Stages
}

func (b *ParallelProcessor) Run(ctx context.Context) (storeMap store.Map, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	initCmd := b.scheduler.Init()
	if err := b.scheduler.Run(ctx, initCmd); err != nil {
		return nil, fmt.Errorf("scheduler run: %w", err)
	}

	if b.reqPlan.LinearPipeline != nil {
		return b.scheduler.FinalStoreMap(b.reqPlan.LinearPipeline.StartBlock)
	}

	return nil, nil
}
