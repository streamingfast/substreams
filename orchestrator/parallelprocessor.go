package orchestrator

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	orchestratorExecout "github.com/streamingfast/substreams/orchestrator/execout"
	"github.com/streamingfast/substreams/orchestrator/responses"
	"github.com/streamingfast/substreams/orchestrator/scheduler"
	"github.com/streamingfast/substreams/orchestrator/squasher"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/pipeline/outputmodules"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/service/config"
	"github.com/streamingfast/substreams/storage"
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
) (*ParallelProcessor, error) {
	// In Dev mode
	// * The linearHandoff will be set to the startblock (never equal to stopBlock, which is exclusive)
	// * We will generate stores up to the linearHandoff, even if we end with an incomplete store
	//
	// In Prod mode
	// * If the stop block is in the irreversible segment (far from chain head), it will be equal to linearHandoff, we stop there.
	// * If the stop block is in the reversible segment (close to chain head), it will higher than linearHandoff, we don't stop there.
	// * If there is no stop block (== 0), the linearHandoff will be at the end of the irreversible segment, we don't stop there
	// * If we stop at the linearHandoff, we will only save the stores up to the boundary of the latest complete store.
	// * On the contrary, if we need to keep going after the linearHandoff, we will need to save the last "incomplete" store.

	stopAtHandoff := reqDetails.LinearHandoffBlockNum == reqDetails.StopBlockNum

	storeLinearHandoffBlockNum := reqDetails.LinearHandoffBlockNum
	if stopAtHandoff {
		// we don't need to bring the stores up to handoff block if we stop there
		storeLinearHandoffBlockNum = lowBoundary(reqDetails.LinearHandoffBlockNum, runtimeConfig.CacheSaveInterval)
	}

	modulesStateMap, err := storage.BuildModuleStorageStateMap( // ok, I will cut stores up to 800 not 842
		ctx,
		storeConfigs,
		runtimeConfig.CacheSaveInterval,
		execoutStorage,
		reqDetails.ResolvedStartBlockNum,
		reqDetails.LinearHandoffBlockNum,
		storeLinearHandoffBlockNum,
	)
	if err != nil {
		return nil, fmt.Errorf("build storage map: %w", err)
	}

	stream := responses.New(respFunc)

	sched := scheduler.New(ctx, stream, outputGraph)

	plan, err := work.BuildNewPlan(ctx, modulesStateMap, runtimeConfig.SubrequestsSplitSize, reqDetails.LinearHandoffBlockNum, runtimeConfig.MaxJobsAhead, outputGraph)
	if err != nil {
		return nil, fmt.Errorf("build work plan: %w", err)
	}
	sched.Planner = plan

	if err := stream.InitialProgressMessages(plan.InitialProgressMessages()); err != nil {
		return nil, fmt.Errorf("initial progress: %w", err)
	}

	if reqDetails.ShouldStreamCachedOutputs() {
		// note: since we are *NOT* in a sub-request and are setting up output module is a map
		requestedModule := outputGraph.OutputModule()
		if requestedModule.GetKindStore() != nil {
			panic("logic error: should not get a store as outputModule on tier 1")
		}

		segmenter := block.NewSegmenter(runtimeConfig.CacheSaveInterval, requestedModule.InitialBlock, reqDetails.LinearHandoffBlockNum)
		walker := execoutStorage.NewFileWalker(requestedModule.Name, segmenter, reqDetails.ResolvedStartBlockNum)

		sched.ExecOutWalker = orchestratorExecout.NewWalker(
			ctx,
			requestedModule,
			walker,
			reqDetails.ResolvedStartBlockNum,
			reqDetails.LinearHandoffBlockNum,
			respFunc, // TODO transform to use `responses` instead, and concentrate all those protobuf manipulations in that package.
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
	//  to dispatch work to process them at all.
	//  -
	//  This is unsolved

	sched.Stages = stage.NewStages(outputGraph, runtimeConfig.SubrequestsSplitSize, reqDetails.LinearHandoffBlockNum)

	// Used to have this param at the end: scheduler.OnStoreCompletedUntilBlock
	// TODO: replace that last param, by the new squashing model in the Scheduler
	squasher, err := squasher.NewMulti(ctx, runtimeConfig, plan.ModulesStateMap, storeConfigs, storeLinearHandoffBlockNum, nil)
	if err != nil {
		return nil, err
	}
	sched.Squasher = squasher

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
