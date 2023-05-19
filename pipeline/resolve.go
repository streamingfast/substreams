package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/dstore"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/reqctx"
	"go.uber.org/zap"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type getBlockFunc func() (uint64, error)

func BuildRequestDetails(
	ctx context.Context,
	request *pbsubstreamsrpc.Request,
	getRecentFinalBlock getBlockFunc,
	resolveCursor CursorResolver,
	getHeadBlock getBlockFunc) (req *reqctx.RequestDetails, undoSignal *pbsubstreamsrpc.BlockUndoSignal, err error) {
	req = &reqctx.RequestDetails{
		Modules:                             request.Modules,
		OutputModule:                        request.OutputModule,
		DebugInitialStoreSnapshotForModules: request.DebugInitialStoreSnapshotForModules,
		ProductionMode:                      request.ProductionMode,
		StopBlockNum:                        request.StopBlockNum,
		UniqueID:                            nextUniqueID(),
	}

	req.ResolvedStartBlockNum, req.ResolvedCursor, undoSignal, err = resolveStartBlockNum(ctx, request, resolveCursor, getHeadBlock)

	if err != nil {
		return nil, nil, err
	}

	linearHandoff, err := computeLiveHandoffBlockNum(request.ProductionMode, req.ResolvedStartBlockNum, request.StopBlockNum, getRecentFinalBlock)
	if err != nil {
		return nil, nil, err
	}

	req.LinearHandoffBlockNum = linearHandoff

	return
}

func BuildRequestDetailsFromSubrequest(request *pbssinternal.ProcessRangeRequest) (req *reqctx.RequestDetails) {
	req = &reqctx.RequestDetails{
		Modules:               request.Modules,
		OutputModule:          request.OutputModule,
		ProductionMode:        true,
		IsSubRequest:          true,
		StopBlockNum:          request.StopBlockNum,
		LinearHandoffBlockNum: request.StopBlockNum,
		ResolvedStartBlockNum: request.StartBlockNum,
		UniqueID:              nextUniqueID(),
	}
	return req
}

var uniqueRequestIDCounter = &atomic.Uint64{}

func nextUniqueID() uint64 {
	return uniqueRequestIDCounter.Add(1)
}

func computeLiveHandoffBlockNum(productionMode bool, startBlock, stopBlock uint64, getRecentFinalBlockFunc func() (uint64, error)) (uint64, error) {
	if productionMode {
		maxHandoff, err := getRecentFinalBlockFunc()
		if err != nil {
			if stopBlock == 0 {
				return 0, fmt.Errorf("cannot determine a recent finalized block: %w", err)
			}
			return stopBlock, nil
		}
		if stopBlock == 0 {
			return maxHandoff, nil
		}
		return minOf(stopBlock, maxHandoff), nil
	}
	maxHandoff, err := getRecentFinalBlockFunc()
	if err != nil {
		return startBlock, nil
	}
	return minOf(startBlock, maxHandoff), nil
}

// resolveStartBlockNum will occasionally modify or remove the cursor inside the request
func resolveStartBlockNum(ctx context.Context, req *pbsubstreamsrpc.Request, resolveCursor CursorResolver, getHeadBlock getBlockFunc) (uint64, string, *pbsubstreamsrpc.BlockUndoSignal, error) {
	// TODO(abourget): a caller will need to verify that, if there's a cursor.Step that is New or Undo,
	// then we need to validate that we are returning not only a number, but an ID,
	// We then need to sync from a known finalized Snapshot's block, down to the potentially
	// forked block in the Cursor, to then send the Substreams Undo payloads to the user,
	// before continuing on to live (or parallel download, if the fork happened way in the past
	// and everything is irreversible.

	if req.StartBlockNum < 0 {
		headBlock, err := getHeadBlock()
		if err != nil {
			return 0, "", nil, fmt.Errorf("resolving negative start block: %w", err)
		}
		req.StartBlockNum = int64(headBlock) + req.StartBlockNum
		if req.StartBlockNum < 0 {
			req.StartBlockNum = 0
		}
	}

	if req.StartCursor == "" {
		return uint64(req.StartBlockNum), "", nil, nil
	}

	cursor, err := bstream.CursorFromOpaque(req.StartCursor)
	if err != nil {
		return 0, "", nil, status.Errorf(grpccodes.InvalidArgument, "invalid StartCursor %q: %s", cursor, err.Error())
	}

	if cursor.IsOnFinalBlock() {
		nextBlock := cursor.Block.Num() + 1
		return nextBlock, "", nil, nil
	}

	reorgJunctionBlock, head, err := resolveCursor(ctx, cursor)
	if err != nil {
		return 0, "", nil, status.Errorf(grpccodes.InvalidArgument, "cannot resolve StartCursor %q: %s", cursor, err.Error())
	}
	var undoSignal *pbsubstreamsrpc.BlockUndoSignal
	resolvedCursor := cursor
	if reorgJunctionBlock != nil && reorgJunctionBlock.Num() != cursor.Block.Num() {
		validCursor := &bstream.Cursor{
			Step:      bstream.StepNew,
			Block:     reorgJunctionBlock,
			LIB:       cursor.LIB,
			HeadBlock: head,
		}
		resolvedCursor = validCursor

		undoSignal = &pbsubstreamsrpc.BlockUndoSignal{
			LastValidBlock:  blockRefToPB(reorgJunctionBlock),
			LastValidCursor: resolvedCursor.ToOpaque(),
		}
	}

	var resolvedStartBlockNum uint64
	switch {
	case resolvedCursor.Step.Matches(bstream.StepNew):
		resolvedStartBlockNum = resolvedCursor.Block.Num() + 1
	case resolvedCursor.Step.Matches(bstream.StepUndo):
		resolvedStartBlockNum = resolvedCursor.Block.Num()
	}

	return resolvedStartBlockNum, resolvedCursor.ToOpaque(), undoSignal, nil
}

type CursorResolver func(context.Context, *bstream.Cursor) (reorgJunctionBlock, currentHead bstream.BlockRef, err error)

type junctionBlockGetter struct {
	reorgJunctionBlock bstream.BlockRef
	currentHead        bstream.BlockRef
}

var Done = errors.New("done")

func (j *junctionBlockGetter) ProcessBlock(block *bstream.Block, obj interface{}) error {
	j.currentHead = obj.(bstream.Cursorable).Cursor().HeadBlock

	stepable := obj.(bstream.Stepable)
	switch {
	case stepable.Step().Matches(bstream.StepNew):
		return Done
	case stepable.Step().Matches(bstream.StepUndo):
		j.reorgJunctionBlock = stepable.ReorgJunctionBlock()
		return Done
	}
	// ignoring other steps
	return nil

}

func NewCursorResolver(hub *hub.ForkableHub, mergedBlocksStore, forkedBlocksStore dstore.Store) CursorResolver {

	return func(ctx context.Context, cursor *bstream.Cursor) (reorgJunctionBlock, currentHead bstream.BlockRef, err error) {
		jctBlkGetter := &junctionBlockGetter{}
		src := hub.SourceFromCursor(cursor, jctBlkGetter)
		if src == nil { // block is out of reversible segment
			src = bstream.NewFileSourceFromCursor(mergedBlocksStore, forkedBlocksStore, cursor, jctBlkGetter, zap.NewNop())
		}

		src.Run()
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-src.Terminated():
		}

		if !errors.Is(src.Err(), Done) {
			headBlock := cursor.HeadBlock
			if headNum, headID, _, _, err := hub.HeadInfo(); err == nil {
				headBlock = bstream.NewBlockRef(headID, headNum)
			}
			return cursor.LIB, headBlock, nil
		}

		return jctBlkGetter.reorgJunctionBlock, jctBlkGetter.currentHead, nil
	}
}

func minOf(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
