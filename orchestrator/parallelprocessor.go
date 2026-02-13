package orchestrator

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/streamingfast/substreams"
	orchestratorExecout "github.com/streamingfast/substreams/orchestrator/execout"
	"github.com/streamingfast/substreams/orchestrator/plan"
	"github.com/streamingfast/substreams/orchestrator/response"
	"github.com/streamingfast/substreams/orchestrator/scheduler"
	"github.com/streamingfast/substreams/orchestrator/stage"
	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/pipeline/exec"
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
	workerPool work.WorkerPool,
	execGraph *exec.Graph,
	execoutStorage *execout.Configs,
	respFunc func(resp substreams.ResponseFromAnyTier) error,
	storeConfigs store.ConfigMap,
	noopMode bool,
	bufferSize int,
	supportBuffering bool,
) (*ParallelProcessor, error) {

	// FIXME: Are all the progress messages properly sent? When we skip some stores and mark them complete,
	// for whatever reason,

	stream := response.New(respFunc)
	sched := scheduler.New(ctx, stream)

	stages := stage.NewStages(ctx, execGraph, reqPlan, execoutStorage, storeConfigs)
	if err := stages.FetchCachesState(ctx); err != nil {
		return nil, fmt.Errorf("fetch caches storage state: %w", err)
	}

	if reqPlan.ReadExecOut != nil {
		// note: since we are *NOT* in a sub-request and are setting up output module is a map
		requestedModule := execGraph.OutputModule()
		if requestedModule.GetKindStore() != nil {
			panic("logic error: should not get a store as outputModule on tier 1")
		}

		// no ReadExecOut if output type is an index
		if requestedModule.GetKindMap() != nil {
			sched.StreamFirstTier2MapSegment = stages.FirstMapperSegmentRequiresProcessing()

			initialBlock := execGraph.ModulesInitBlocks()[requestedModule.Name]
			if execOutSegmenter := reqPlan.ReadOutSegmenter(initialBlock, sched.StreamFirstTier2MapSegment); execOutSegmenter != nil {
				walker := execoutStorage.NewFileWalker(requestedModule.Name, execOutSegmenter)

				sched.ExecOutWalker = orchestratorExecout.NewWalker(
					ctx,
					requestedModule,
					walker,
					reqPlan.ReadExecOut,
					stream,
					noopMode,
					bufferSize,
					supportBuffering,
				)
			}
		}
	}

	sched.Stages = stages

	if os.Getenv("SUBSTREAMS_DEBUG_SCHEDULER_STATE") == "true" {
		fmt.Println("Initial state:")
		fmt.Print(stages.StatesString())
	}

	sched.WorkerPool = workerPool

	return &ParallelProcessor{
		scheduler: sched,
		reqPlan:   reqPlan,
	}, nil
}

func (b *ParallelProcessor) Stages() *stage.Stages {
	return b.scheduler.Stages
}

func (b *ParallelProcessor) Run(ctx context.Context, checkPendingShutdown func() bool) (storeMap store.Map, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		for {
			select {
			case <-time.Tick(time.Second):
				if checkPendingShutdown() {
					b.scheduler.Send(work.MsgPendingShutdown{})
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	initCmd := b.scheduler.Init()
	if err := b.scheduler.Run(ctx, initCmd); err != nil {
		return nil, fmt.Errorf("scheduler run: %w", err)
	}

	if b.reqPlan.LinearPipeline != nil {
		return b.scheduler.FinalStoreMap(b.reqPlan.LinearPipeline.StartBlock)
	}

	return nil, nil
}
