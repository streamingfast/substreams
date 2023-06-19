package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	orchestratorExecout "github.com/streamingfast/substreams/orchestrator/execout"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/scheduler"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/storage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
)

type ParallelProcessor struct {
	scheduler *scheduler.Scheduler
}

// BuildParallelProcessor is only called on tier1
func BuildParallelProcessor(
	ctx context.Context,
	reqDetails *reqctx.RequestDetails,
	runtimeConfig config.RuntimeConfig,
	outputGraph *outputmodules.Graph,
	execoutStorage *execout.Configs,
	respFunc func(resp substreams.ResponseFromAnyTier) error,
	storeConfigs store.ConfigMap,
	traceID string,
) (*ParallelProcessor, error) {
	stream := response.New(respFunc)
	sched := scheduler.New(ctx, stream, outputGraph)

	// Job segmenter really.. stores should just match this
	plan := plan.BuildRequestPlan(
		reqDetails.ProductionMode,
		runtimeConfig.SubrequestsSplitSize,
		outputGraph.LowestInitBlock(),
		reqDetails.ResolvedStartBlockNum,
		reqDetails.LinearHandoffBlockNum,
		reqDetails.StopBlockNum,
	)

	storesSegmenter := plan.StoresSegmenter()
	sched.Stages = stage.NewStages(ctx, outputGraph, storesSegmenter, storeConfigs, traceID)

	storageState, err := storage.FetchStoresState(
		ctx,
		sched.Stages,
		storesSegmenter,
		storeConfigs,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch stores storage state: %w", err)
	}
	if err := stream.InitialProgressMessages(storageState); err != nil {
		return nil, fmt.Errorf("initial progress: %w", err)
	}

	// TODO: inject the `storageState` into the Scheduler with a bunch of messages
	// or pass it through some sort of `Init()` mechanism.

	// TODO(abourget): I want to transform this to use a Segmenter
	// but I'm not sure I can use the exact same Segmenter as the
	// scheduler. This one
	//modulesStateMap, err := storage.BuildModuleStorageStateMap( // ok, I will cut stores up to 800 not 842
	//	ctx,
	//	storeConfigs,
	//	runtimeConfig.CacheSaveInterval,
	//	execoutStorage,
	//	reqDetails.ResolvedStartBlockNum,
	//	reqDetails.LinearHandoffBlockNum,
	//	storeLinearHandoffBlockNum,
	//)
	//if err != nil {
	//	return nil, fmt.Errorf("build storage map: %w", err)
	//}
	// FIXME: Is the state map the final reference for the progress we've made?
	// Shouldn't that be processed by the scheduler a little bit?
	// What if we have discovered a bunch of ExecOut files and the scheduler
	// would decide not to use the very first stores as a sign of what is complete?
	// Well, perhaps those wouldn't hurt, because here we're _sure_ they're
	// done and the Scheduler could send Progress messages when the above decision
	// is taken.

	execOutSegmenter := plan.WriteOutSegmenter()
	if execOutSegmenter != nil {
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
			reqDetails.ResolvedStartBlockNum,
			reqDetails.LinearHandoffBlockNum,
			respFunc, // TODO: transform to use `stream` instead, and concentrate all those protobuf manipulations in that package.
		)
	}

	// TODO(abourget): take all of the ExecOut files that exist
	//  and use that to PUSH back what the Stages need to do.
	//  So the first Segment to process will not necessarily be
	//  segment == 0.  We'll need the segment JUST prior to be
	//  processed though, because we need to continue working on
	//  the future segments.  This interplays with the segment
	//  just before.
	//  -
	//  If we can stream out the ExecOut directly, we don't need
	//  to dispatch work to process them at all.  But we'll need
	//  to have stores ready to continue segments work.
	//  SO: we can move forward the processing pipeline, provided
	//  all of the stages can be continued forward after the
	//  last ExecOut segment: that we have complete stores for the
	//  segment where ExecOut finishes.
	//  -
	//  This is unsolved

	// TODO(abourget): validate here the last parameter used
	// should it be `storeLinearHandoff` as defined above, or
	// reqDetails.LinearHandoffBlockNum really?

	// Used to have this param at the end: scheduler.OnStoreCompletedUntilBlock
	// TODO: replace that last param, by the new squashing model in the Scheduler
	//squasher, err := squasher.NewMulti(ctx, segmenter, runtimeConfig, modulesStateMap, storeConfigs, storeLinearHandoffBlockNum, nil)
	//if err != nil {
	//	return nil, err
	//}
	//sched.Squasher = squasher

	// TODO: wrap up the tying up of the Scheduler with the Squasher.
	//scheduler.OnStoreJobTerminated = squasher.Squash

	workerPool := work.NewWorkerPool(ctx, int(runtimeConfig.ParallelSubrequests), runtimeConfig.WorkerFactory)
	sched.WorkerPool = workerPool

	return &ParallelProcessor{
		scheduler: sched,
	}, nil
}

func (b *ParallelProcessor) Run(ctx context.Context) (storeMap store.Map, err error) {
	b.scheduler.Init()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := b.scheduler.Run(ctx); err != nil {
		return nil, fmt.Errorf("scheduler run: %w", err)
	}

	storeMap = b.scheduler.FinalStoreMap()

	// TODO: this needs to be handled by the completion Shutdown
	// processes of the new Scheduler:
	//
	//if b.execOutputReader != nil {
	//	select {
	//	case <-b.execOutputReader.Terminated():
	//		if err := b.execOutputReader.Err(); err != nil {
	//			return nil, err
	//		}
	//	case <-ctx.Done():
	//	}
	//}

	return storeMap, nil
}

func lowBoundary(blk uint64, bundleSize uint64) uint64 {
	return blk - (blk % bundleSize)
}
