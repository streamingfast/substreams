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
	"github.com/streamingfast/substreams/service/config"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

type LinearExecOutputReader struct {
	*shutter.Shutter
	startBlock        uint64
	exclusiveEndBlock uint64
	responseFunc      substreams.ResponseFunc
	logger            *zap.Logger
	cfg               config.RuntimeConfig
	runtimeConfig     *config.RuntimeConfig
	module            *pbsubstreams.Module
	cache             *execout.File
}

func NewLinearExecOutputReader(startBlock uint64, exclusiveEndBlock uint64, module *pbsubstreams.Module, cache *execout.File, responseFunc substreams.ResponseFunc, runtimeConfig *config.RuntimeConfig, logger *zap.Logger) *LinearExecOutputReader {
	logger = logger.With(zap.String("component", "downloader"))
	logger.Info("creating downloader", zap.Uint64("start_block", startBlock), zap.Uint64("exclusive_end_block", exclusiveEndBlock))
	return &LinearExecOutputReader{
		startBlock:        startBlock,
		exclusiveEndBlock: exclusiveEndBlock,
		module:            module,
		cache:             cache,
		responseFunc:      responseFunc,
		runtimeConfig:     runtimeConfig,
		logger:            logger,
	}
}

func (r *LinearExecOutputReader) Launch(ctx context.Context) {
	stream := NewCachedItemStream(r.startBlock, r.module, r.cache, r.runtimeConfig, r.logger)
	r.OnTerminating(func(err error) {
		stream.Shutdown(err)
	})
	stream.Launch(ctx)

	go func() {
		r.Shutdown(r.run(ctx, stream))
	}()
}

func (r *LinearExecOutputReader) run(ctx context.Context, stream *CachedItemStream) error {
	for {
		cacheItem, err := stream.next(ctx)
		if cacheItem == nil {
			return nil
		}

		blockScopedData, err := toBlockScopedData(r.module, cacheItem)
		err = r.responseFunc(substreams.NewBlockScopedDataResponse(blockScopedData))
		if err != nil {
			return fmt.Errorf("calling response func: %w", err)
		}
		if blockScopedData.Clock.Number >= r.exclusiveEndBlock {
			r.logger.Info("stop pulling block scoped data, end block reach",
				zap.Uint64("exclusive_end_block_num", r.exclusiveEndBlock),
				zap.Uint64("cache_item_block_num", blockScopedData.Clock.Number),
			)
			break
		}
	}
	return nil
}

type CachedItemStream struct {
	*shutter.Shutter
	module            *pbsubstreams.Module
	cache             *execout.File
	requestStartBlock uint64
	logger            *zap.Logger
	runtimeConfig     *config.RuntimeConfig
	cacheItems        chan *pboutput.Item
}

func NewCachedItemStream(requestStartBlock uint64, module *pbsubstreams.Module, cache *execout.File, runtimeConfig *config.RuntimeConfig, logger *zap.Logger) *CachedItemStream {
	return &CachedItemStream{
		requestStartBlock: requestStartBlock,
		module:            module,
		cache:             cache,
		logger:            logger,
		runtimeConfig:     runtimeConfig,
		cacheItems:        make(chan *pboutput.Item, runtimeConfig.ExecOutputSaveInterval*2),
	}
}

func (s *CachedItemStream) Launch(ctx context.Context) {
	go func() {
		s.Shutdown(s.run(ctx))
	}()
}

func (s *CachedItemStream) next(ctx context.Context) (*pboutput.Item, error) {
	select {
	case <-ctx.Done():
		return nil, nil
	case <-s.Terminating():
		return nil, nil
	case item := <-s.cacheItems:
		return item, nil
	}
}

func (s *CachedItemStream) run(ctx context.Context) error {
	nextCachedBlockNum := s.requestStartBlock - (s.requestStartBlock % s.runtimeConfig.ExecOutputSaveInterval)
	for {
		sortedCachedItems, err := s.sortedCacheItems(ctx, nextCachedBlockNum)
		if err != nil {
			return fmt.Errorf("getting sorted cache items: %w", err)
		}

		if len(sortedCachedItems) == 0 {
			return nil
		}

		nextCachedBlockNum += s.runtimeConfig.ExecOutputSaveInterval
		for _, cachedItem := range sortedCachedItems {
			select {
			case s.cacheItems <- cachedItem:
				continue
			case <-s.Terminating():
				return nil
			case <-ctx.Done():
				return nil
			default:
			}
		}
	}
}

func (s *CachedItemStream) sortedCacheItems(ctx context.Context, atBlockNum uint64) (out []*pboutput.Item, err error) {
	for {
		s.logger.Debug("loading next cache", zap.String("module", s.module.Name), zap.Uint64("next_cached_block_num", atBlockNum))
		found, err := s.cache.LoadAtBlock(ctx, atBlockNum)
		if err != nil {
			return nil, fmt.Errorf("loading %s cache at block %d: %w", s.module.Name, atBlockNum, err)
		}
		if !found {
			s.logger.Debug("cache not found, waiting 5s", zap.String("module", s.module.Name), zap.Uint64("next_cached_block_num", atBlockNum))
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-s.Terminating():
				return nil, nil
			case <-ctx.Done():
				return nil, nil
			}
		}
		out = s.cache.SortedCacheItems()
		return out, nil
	}
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
