package service

import (
	"context"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/metering"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pipeline"
	"google.golang.org/protobuf/proto"
)

// EstimateHandler processes blocks in estimate mode without persisting data,
// collecting only billing metrics (egress bytes and processed blocks) for cost estimation.
type EstimateHandler struct {
	respFunc substreams.ResponseFunc
	ctx      context.Context
}

// NewEstimateHandler creates a new EstimateHandler instance.
func NewEstimateHandler(ctx context.Context, respFunc substreams.ResponseFunc) *EstimateHandler {
	return &EstimateHandler{
		respFunc: respFunc,
		ctx:      ctx,
	}
}

// ProcessBlock processes a block in estimate mode, collecting metrics without persisting data.
func (e *EstimateHandler) ProcessBlock(blk *pbbstream.Block, obj any) (err error) {
	clock := pipeline.BlockToClock(blk)

	step := obj.(bstream.Stepable).Step()
	switch step {
	case bstream.StepIrreversible,
		bstream.StepNewIrreversible,
		bstream.StepNew:
	default:
		return nil
	}

	cursor := &bstream.Cursor{
		Step:      step,
		Block:     bstream.NewBlockRef(clock.Id, clock.Number),
		LIB:       bstream.NewBlockRef(clock.Id, clock.Number),
		HeadBlock: bstream.NewBlockRef(clock.Id, clock.Number),
	}

	// Create a response similar to NoopHandler but collect metrics
	out := &pbsubstreamsrpc.BlockScopedData{
		Clock: clock,
		Output: &pbsubstreamsrpc.MapModuleOutput{
			MapOutput: nil,
		},
		Cursor:           cursor.ToOpaque(),
		FinalBlockHeight: cursor.LIB.Num(),
	}

	// Calculate egress bytes for billing metrics
	egressBytes := proto.Size(out)

	// Record billing metrics for cost estimation
	metering.AddEgressBytes(e.ctx, egressBytes)
	metering.AddProcessedBlocks(e.ctx, 1)

	return e.respFunc(substreams.NewBlockScopedDataResponse(out))
}
