package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/streamingfast/substreams"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout/cachev1"
	"github.com/streamingfast/substreams/service/config"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

type Downloader struct {
	startBlock        uint64
	exclusiveEndBlock uint64
	responseFunc      substreams.ResponseFunc
	logger            *zap.Logger
	cfg               config.RuntimeConfig
	runtimeConfig     *config.RuntimeConfig
}

func NewDownloader(startBlock uint64, exclusiveEndBlock uint64, responseFunc substreams.ResponseFunc, runtimeConfig *config.RuntimeConfig, logger *zap.Logger) *Downloader {
	logger = logger.With(zap.String("component", "downloader"))
	logger.Info("creating downloader", zap.Uint64("start_block", startBlock), zap.Uint64("exclusive_end_block", exclusiveEndBlock))
	return &Downloader{
		responseFunc:      responseFunc,
		startBlock:        startBlock,
		exclusiveEndBlock: exclusiveEndBlock,
		runtimeConfig:     runtimeConfig,
		logger:            logger,
	}
}

func (d *Downloader) Run(ctx context.Context, module *pbsubstreams.Module, cache *cachev1.OutputCache) error {
	stream := NewCachedItemStream(d.startBlock, module, cache, d.runtimeConfig, d.logger)
	for {
		cacheItem, err := stream.next(ctx)
		blockScopedData, err := toBlockScopedData(module, cacheItem)
		err = d.responseFunc(substreams.NewBlockScopedDataResponse(blockScopedData))
		if err != nil {
			return fmt.Errorf("calling response func: %w", err)
		}
		if blockScopedData.Clock.Number >= d.exclusiveEndBlock {
			d.logger.Info("stop pulling block scoped data, end block reach",
				zap.Uint64("exclusive_end_block_num", d.exclusiveEndBlock),
				zap.Uint64("cache_item_block_num", blockScopedData.Clock.Number),
			)
			break
		}
	}

	return nil
}

type CachedItemStream struct {
	nextCachedBlockNum uint64
	module             *pbsubstreams.Module
	cache              *cachev1.OutputCache
	requestStartBlock  uint64
	sortedCacheItems   []*cachev1.CacheItem
	logger             *zap.Logger
	runtimeConfig      *config.RuntimeConfig
}

func NewCachedItemStream(requestStartBlock uint64, module *pbsubstreams.Module, cache *cachev1.OutputCache, runtimeConfig *config.RuntimeConfig, logger *zap.Logger) *CachedItemStream {
	return &CachedItemStream{
		requestStartBlock:  requestStartBlock,
		nextCachedBlockNum: requestStartBlock,
		module:             module,
		cache:              cache,
		logger:             logger,
		runtimeConfig:      runtimeConfig,
	}
}

func (s *CachedItemStream) next(ctx context.Context) (*cachev1.CacheItem, error) {
	for {
		if len(s.sortedCacheItems) > 0 {
			break
		}
		err := s.loadNextCache(ctx)
		if err != nil {
			return nil, fmt.Errorf("loading next cache: %w", err)
		}
	}
	cacheItem := s.sortedCacheItems[0]
	s.sortedCacheItems = s.sortedCacheItems[1:]
	return cacheItem, nil
}

func (s *CachedItemStream) loadNextCache(ctx context.Context) (err error) {
	for {
		s.logger.Debug("loading next cache", zap.String("module", s.module.Name), zap.Uint64("next_cached_block_num", s.nextCachedBlockNum))
		found, err := s.cache.LoadAtBlock(ctx, s.nextCachedBlockNum)
		if err != nil {
			return fmt.Errorf("loading %s cache at block %d: %w", s.module.Name, s.nextCachedBlockNum, err)
		}

		if !found {
			s.logger.Debug("cache not found, waiting 5s", zap.String("module", s.module.Name), zap.Uint64("next_cached_block_num", s.nextCachedBlockNum))
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return nil
			}
		}
		s.nextCachedBlockNum += s.runtimeConfig.ExecOutputSaveInterval
		s.sortedCacheItems = s.cache.SortedCacheItems()
		return nil
	}
}

func toModuleOutput(module *pbsubstreams.Module, cacheItem *cachev1.CacheItem) (*pbsubstreams.ModuleOutput, error) {
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

func toClock(item *cachev1.CacheItem) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{
		Id:        item.BlockID,
		Number:    item.BlockNum,
		Timestamp: item.Timestamp,
	}
}

func toBlockScopedData(module *pbsubstreams.Module, cacheItem *cachev1.CacheItem) (*pbsubstreams.BlockScopedData, error) {
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
