package execout

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
)

type Walker struct {
	ctx               context.Context
	requestStartBlock uint64
	exclusiveEndBlock uint64
	fileWalker        *execout.FileWalker
	streamOut         *response.Stream
	module            *pbsubstreams.Module
	logger            *zap.Logger
}

func NewWalker(
	ctx context.Context,
	module *pbsubstreams.Module,
	fileWalker *execout.FileWalker,
	startBlock uint64,
	exclusiveEndBlock uint64,
	stream *response.Stream,

) *Walker {
	logger := reqctx.Logger(ctx)
	return &Walker{
		ctx:               ctx,
		module:            module,
		fileWalker:        fileWalker,
		requestStartBlock: startBlock,
		exclusiveEndBlock: exclusiveEndBlock,
		streamOut:         stream,
		logger:            logger,
	}
}

func (r *Walker) CmdDownloadCurrentSegment(waitBefore time.Duration) loop.Cmd {
	if r.IsCompleted() {
		return func() loop.Msg {
			return MsgWalkerCompleted{}
		}
	}

	file := r.fileWalker.File()
	return func() loop.Msg {
		time.Sleep(waitBefore)

		err := file.Load(r.ctx)
		if err == dstore.ErrNotFound {
			return MsgFileNotPresent{}
		}
		if err != nil {
			return loop.Quit(fmt.Errorf("loading %s cache %q: %w", file.ModuleName, file.Filename(), err))
		}

		if err := r.sendItems(file.SortedItems()); err != nil {
			return loop.Quit(err)
		}
		return MsgFileDownloaded{}
	}
}

func (r *Walker) sendItems(sortedItems []*pboutput.Item) error {
	for _, item := range sortedItems {
		if item == nil {
			continue // why would that happen?!
		}
		if item.BlockNum < r.requestStartBlock {
			continue
		}

		blockScopedData, err := toBlockScopedData(r.module, item)
		if err != nil {
			return fmt.Errorf("converting to block scoped data: %w", err)
		}

		if err = r.streamOut.BlockScopedData(blockScopedData); err != nil {
			return fmt.Errorf("calling response func: %w", err)
		}

		if blockScopedData.Clock.Number >= r.exclusiveEndBlock {
			r.logger.Info("stop pulling block scoped data, end block reach",
				zap.Uint64("exclusive_end_block_num", r.exclusiveEndBlock),
				zap.Uint64("cache_item_block_num", blockScopedData.Clock.Number),
			)
			return nil
		}
	}
	return nil
}

func (r *Walker) NextSegment() {
	r.fileWalker.Next()
}

func (r *Walker) IsCompleted() bool {
	return r.fileWalker.IsDone()
}

func toBlockScopedData(module *pbsubstreams.Module, cacheItem *pboutput.Item) (*pbsubstreamsrpc.BlockScopedData, error) {
	clock := toClock(cacheItem)
	blockRef := bstream.NewBlockRef(clock.Id, clock.Number)
	cursor := bstream.Cursor{
		Step:      bstream.StepNewIrreversible,
		Block:     blockRef,
		LIB:       blockRef,
		HeadBlock: blockRef,
	}
	out := &pbsubstreamsrpc.BlockScopedData{
		Cursor:           cursor.ToOpaque(),
		Clock:            clock,
		FinalBlockHeight: blockRef.Num(),
	}

	m, err := toModuleOutput(module, cacheItem)
	if err != nil {
		return nil, fmt.Errorf("module output: %w", err)
	}
	out.Output = m

	return out, nil
}

func toModuleOutput(module *pbsubstreams.Module, cacheItem *pboutput.Item) (*pbsubstreamsrpc.MapModuleOutput, error) {
	outputType := strings.TrimPrefix(module.Output.Type, "proto:")

	return &pbsubstreamsrpc.MapModuleOutput{
		Name:      module.Name,
		MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + outputType, Value: cacheItem.Payload},
	}, nil
}

func toClock(item *pboutput.Item) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{
		Id:        item.BlockId,
		Number:    item.BlockNum,
		Timestamp: item.Timestamp,
	}
}
