package service

import (
	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/pipeline"
)

type NoopHandler struct {
	respFunc substreams.ResponseFunc
}

func NewNoopHandler(respFunc substreams.ResponseFunc) *NoopHandler {
	return &NoopHandler{
		respFunc: respFunc,
	}
}

func (n NoopHandler) ProcessBlock(blk *pbbstream.Block, obj interface{}) (err error) {
	clock := pipeline.BlockToClock(blk)

	step := obj.(bstream.Stepable).Step()

	cursor := &bstream.Cursor{
		Step:      step,
		Block:     bstream.NewBlockRef(clock.Id, clock.Number),
		LIB:       bstream.NewBlockRef(clock.Id, clock.Number),
		HeadBlock: bstream.NewBlockRef(clock.Id, clock.Number),
	}

	out := &pbsubstreamsrpc.BlockScopedData{
		Clock:            clock,
		Output:           &pbsubstreamsrpc.MapModuleOutput{},
		Cursor:           cursor.ToOpaque(),
		FinalBlockHeight: cursor.LIB.Num(),
	}

	return n.respFunc(substreams.NewBlockScopedDataResponse(out))
}
