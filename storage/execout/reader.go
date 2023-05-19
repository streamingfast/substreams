package execout

import (
	"context"
	"fmt"
	"github.com/streamingfast/dstore"
	"strings"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/shutter"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
)

type LinearReader struct {
	*shutter.Shutter
	requestStartBlock  uint64
	exclusiveEndBlock  uint64
	responseFunc       substreams.ResponseFunc
	pendingUndoMessage *pbsubstreamsrpc.Response
	module             *pbsubstreams.Module
	firstFile          *File
	cacheItems         chan *pboutput.Item
}

func NewLinearReader(
	startBlock uint64,
	exclusiveEndBlock uint64,
	module *pbsubstreams.Module,
	firstFile *File,
	responseFunc substreams.ResponseFunc,
	execOutputSaveInterval uint64,
	pendingUndoMessage *pbsubstreamsrpc.Response,
) *LinearReader {
	return &LinearReader{
		Shutter:            shutter.New(),
		requestStartBlock:  startBlock,
		exclusiveEndBlock:  exclusiveEndBlock,
		module:             module,
		firstFile:          firstFile,
		responseFunc:       responseFunc,
		pendingUndoMessage: pendingUndoMessage,
		cacheItems:         make(chan *pboutput.Item, execOutputSaveInterval*2),
	}
}

func (r *LinearReader) Launch(ctx context.Context) {
	logger := reqctx.Logger(ctx)
	logger.Info("launching downloader", zap.Uint64("start_block", r.requestStartBlock), zap.Uint64("exclusive_end_block", r.exclusiveEndBlock))

	go func() {
		ctx, span := reqctx.WithSpan(ctx, "substreams/tier1/pipeline/linear_reader")
		err := r.run(ctx)
		span.EndWithErr(&err)
		r.Shutdown(err)
	}()
}

func (r *LinearReader) run(ctx context.Context) error {
	logger := reqctx.Logger(ctx)

	go func() {
		if err := r.download(ctx, r.firstFile); err != nil {
			r.Shutdown(err)
		}
		close(r.cacheItems)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.Terminating():
			return nil
		case item := <-r.cacheItems:
			if item == nil {
				return nil
			}
			if item.BlockNum < r.requestStartBlock {
				continue
			}

			if r.pendingUndoMessage != nil {
				if err := r.responseFunc(r.pendingUndoMessage); err != nil {
					return fmt.Errorf("sending undo message: %w", err)
				}
				r.pendingUndoMessage = nil
			}

			blockScopedData, err := toBlockScopedData(r.module, item)
			err = r.responseFunc(substreams.NewBlockScopedDataResponse(blockScopedData))
			if err != nil {
				return fmt.Errorf("calling response func: %w", err)
			}

			if blockScopedData.Clock.Number >= r.exclusiveEndBlock {
				logger.Info("stop pulling block scoped data, end block reach",
					zap.Uint64("exclusive_end_block_num", r.exclusiveEndBlock),
					zap.Uint64("cache_item_block_num", blockScopedData.Clock.Number),
				)
				return nil
			}
		}
	}
}

func (r *LinearReader) download(ctx context.Context, file *File) error {
	for {
		sortedCachedItems, err := r.downloadFile(ctx, file)
		if err != nil {
			return fmt.Errorf("getting sorted cache items: %w", err)
		}

		for _, cachedItem := range sortedCachedItems {
			select {
			case r.cacheItems <- cachedItem:
			case <-r.Terminating():
				return nil
			case <-ctx.Done():
				return nil
			}
		}

		file = file.NextFile()
		if file == nil {
			return nil
		}
	}
}

func (r *LinearReader) downloadFile(ctx context.Context, file *File) ([]*pboutput.Item, error) {
	logger := reqctx.Logger(ctx)
	for {
		logger.Debug("loading next cache", zap.Object("file", file))

		err := file.Load(ctx)
		if err != nil && err != dstore.ErrNotFound {
			return nil, fmt.Errorf("loading %s cache %q: %w", file.ModuleName, file.Filename(), err)
		}

		// err can be equal to dstore.ErrNotFound, we want to skip this and retur
		if err == nil {
			return file.SortedItems(), nil
		}

		// TODO(abourget): if file.IsPartial(), we should delete it, it would mean it'd be left
		// over, and never reused, unless an EXACT request would come and use it.

		logger.Debug("cache not found, waiting 2s", zap.Object("file", file))
		select {
		case <-time.After(2 * time.Second):
			continue
		case <-r.Terminating():
			return nil, nil
		case <-ctx.Done():
			return nil, nil
		}
	}
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
