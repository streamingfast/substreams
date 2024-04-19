package pipeline

import (
	"context"

	"github.com/streamingfast/bstream"

	"github.com/streamingfast/substreams/reqctx"
)

type gate struct {
	startBlockNum uint64
	disabled      bool
	passed        bool
	snapshotSent  bool
}

func newGate(ctx context.Context) *gate {
	reqDetails := reqctx.Details(ctx)
	return &gate{
		disabled:      reqDetails.IsTier2Request,
		startBlockNum: reqDetails.LinearGateBlockNum,
	}
}

func (g *gate) processBlock(blockNum uint64, step bstream.StepType) {
	if g.disabled || g.passed {
		return
	}

	if blockTriggersGate(blockNum, g.startBlockNum, step) {
		g.passed = true
	}
}

func (g *gate) shouldSendSnapshot() bool {
	if g.snapshotSent {
		return false
	}

	if g.passed {
		g.snapshotSent = true
		return true
	}
	return false
}

func (g *gate) shouldSendOutputs() bool {
	return g.passed
}

func blockTriggersGate(blockNum, requestStartBlockNum uint64, step bstream.StepType) bool {
	if step.Matches(bstream.StepNew) {
		return blockNum >= requestStartBlockNum
	}
	if step.Matches(bstream.StepUndo) {
		return true
	}
	return false
}
