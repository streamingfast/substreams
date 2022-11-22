package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/substreams/storage/execout"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"

	"github.com/streamingfast/shutter"
	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

type LinearExecOutputReader struct {
	*shutter.Shutter
	requestStartBlock      uint64
	exclusiveEndBlock      uint64
	responseFunc           substreams.ResponseFunc
	logger                 *zap.Logger
	execOutputSaveInterval uint64
	module                 *pbsubstreams.Module
	cache                  *execout.File
	cacheItems             chan *pboutput.Item
}

func NewLinearExecOutputReader(startBlock uint64, exclusiveEndBlock uint64, module *pbsubstreams.Module, cache *execout.File, responseFunc substreams.ResponseFunc, execOutputSaveInterval uint64, logger *zap.Logger) *LinearExecOutputReader {
	logger = logger.With(zap.String("component", "downloader"))
	logger.Info("creating downloader", zap.Uint64("start_block", startBlock), zap.Uint64("exclusive_end_block", exclusiveEndBlock))
	return &LinearExecOutputReader{
		Shutter:                shutter.New(),
		requestStartBlock:      startBlock,
		exclusiveEndBlock:      exclusiveEndBlock,
		module:                 module,
		cache:                  cache,
		responseFunc:           responseFunc,
		execOutputSaveInterval: execOutputSaveInterval,
		logger:                 logger,
		cacheItems:             make(chan *pboutput.Item, execOutputSaveInterval*2),
	}
}

func (r *LinearExecOutputReader) Launch(ctx context.Context) {
	go func() {
		r.Shutdown(r.run(ctx))
	}()
}

func (r *LinearExecOutputReader) run(ctx context.Context) error {
	go func() {
		r.Shutdown(r.download(ctx))
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

			blockScopedData, err := toBlockScopedData(r.module, item)
			err = r.responseFunc(substreams.NewBlockScopedDataResponse(blockScopedData))
			if err != nil {
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
	}
}

func (r *LinearExecOutputReader) download(ctx context.Context) error {
	nextCachedBlockNum := r.requestStartBlock - (r.requestStartBlock % r.execOutputSaveInterval)
	for {
		sortedCachedItems, err := r.downloadNextFile(ctx, nextCachedBlockNum)
		if err != nil {
			return fmt.Errorf("getting sorted cache items: %w", err)
		}

		if len(sortedCachedItems) == 0 {
			return nil
		}

		nextCachedBlockNum += r.execOutputSaveInterval
		for _, cachedItem := range sortedCachedItems {
			select {
			case r.cacheItems <- cachedItem:
				continue
			case <-r.Terminating():
				return nil
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (r *LinearExecOutputReader) downloadNextFile(ctx context.Context, atBlockNum uint64) (out []*pboutput.Item, err error) {
	for {
		r.logger.Debug("loading next cache", zap.String("module", r.module.Name), zap.Uint64("next_cached_block_num", atBlockNum))
		found, err := r.cache.LoadAtBlock(ctx, atBlockNum)
		if err != nil {
			return nil, fmt.Errorf("loading %s cache at block %d: %w", r.module.Name, atBlockNum, err)
		}
		if !found {
			r.logger.Debug("cache not found, waiting 5s", zap.String("module", r.module.Name), zap.Uint64("next_cached_block_num", atBlockNum))
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-r.Terminating():
				return nil, nil
			case <-ctx.Done():
				return nil, nil
			}
		}
		out = r.cache.SortedCacheItems()
		return out, nil
	}
}

func toBlockScopedData(module *pbsubstreams.Module, cacheItem *pboutput.Item) (*pbsubstreams.BlockScopedData, error) {
	out := &pbsubstreams.BlockScopedData{
		Step: pbsubstreams.ForkStep_STEP_IRREVERSIBLE,
	}

	out.Clock = toClock(cacheItem)
	out.Cursor = cacheItem.Cursor
	m, err := toModuleOutput(module, cacheItem)
	if err != nil {
		return nil, fmt.Errorf("module output: %w", err)
	}
	out.Outputs = append(out.Outputs, m)

	return out, nil
}

func toModuleOutput(module *pbsubstreams.Module, cacheItem *pboutput.Item) (*pbsubstreams.ModuleOutput, error) {
	var output pbsubstreams.ModuleOutputData
	switch module.Kind.(type) {
	case *pbsubstreams.Module_KindMap_:
		output = &pbsubstreams.ModuleOutput_MapOutput{
			MapOutput: &anypb.Any{
				TypeUrl: "type.googleapis.com/" + module.Output.Type,
				Value:   cacheItem.Payload,
			},
		}
	default:
		panic(fmt.Sprintf("invalid module file %T", module.Kind))
	}

	return &pbsubstreams.ModuleOutput{
		Name: module.Name,
		Data: output,
	}, nil
}

func toClock(item *pboutput.Item) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{
		Id:        item.BlockId,
		Number:    item.BlockNum,
		Timestamp: item.Timestamp,
	}
}
