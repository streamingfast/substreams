package state

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/outputs"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func (b *Builder) InitializePartial(ctx context.Context, startBlock uint64) error {
	b.partialMode = true
	b.BlockRange = &block.Range{
		StartBlock:        startBlock,
		ExclusiveEndBlock: startBlock + b.saveInterval,
	}

	fileName := PartialFileName(b.BlockRange)
	return b.loadState(ctx, fileName)
}

func (b *Builder) Initialize(ctx context.Context, requestedStartBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	b.BlockRange.StartBlock = b.ModuleStartBlock

	zlog.Debug("initializing builder", zap.String("module_name", b.Name), zap.Uint64("requested_start_block", requestedStartBlock))
	floor := requestedStartBlock - requestedStartBlock%b.saveInterval
	if requestedStartBlock == b.BlockRange.StartBlock {
		b.BlockRange.StartBlock = requestedStartBlock
		b.BlockRange.ExclusiveEndBlock = floor + b.saveInterval
		b.KV = map[string][]byte{}
		return nil
	}

	deltasStartBlock := uint64(0)

	zlog.Debug("computed info", zap.String("module_name", b.Name), zap.Uint64("start_block", floor))

	deltasNeeded := false
	if floor >= b.saveInterval && floor > b.BlockRange.StartBlock {
		deltasStartBlock = requestedStartBlock - floor
		deltasNeeded = deltasStartBlock > 0

		atBlock := floor - b.saveInterval // get the previous saved range
		b.BlockRange.ExclusiveEndBlock = floor
		fileName := FullStateFileName(&block.Range{
			StartBlock:        b.ModuleStartBlock,
			ExclusiveEndBlock: b.BlockRange.ExclusiveEndBlock,
		})

		zlog.Info("about to load state", zap.String("module_name", b.Name), zap.Uint64("at_block", atBlock), zap.Uint64("deltas_start_block", deltasStartBlock))
		err := b.loadState(ctx, fileName)
		if err != nil {
			return fmt.Errorf("reading state file for module %q: %w", b.Name, err)
		}
	} else {
		deltasNeeded = true
		deltasStartBlock = b.BlockRange.StartBlock
		b.BlockRange.ExclusiveEndBlock = floor + b.saveInterval
	}

	if deltasNeeded {
		err := b.loadDelta(ctx, deltasStartBlock, requestedStartBlock, outputCacheSaveInterval, outputCacheStore)
		if err != nil {
			return fmt.Errorf("loading delta for builder %q: %w", b.Name, err)
		}
	}

	return nil
}

func (b *Builder) loadState(ctx context.Context, stateFileName string) error {
	zlog.Debug("loading state from file", zap.String("module_name", b.Name), zap.String("file_name", stateFileName))

	r, err := b.Store.OpenObject(ctx, stateFileName)
	if err != nil {
		return fmt.Errorf("opening file state file %s: %w", stateFileName, err)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading data: %w", err)
	}
	defer r.Close()

	kv := map[string]string{}
	if err = json.Unmarshal(data, &kv); err != nil {
		return fmt.Errorf("json unmarshal of state file %s data: %w", stateFileName, err)
	}

	b.KV = byteMap(kv)

	zlog.Debug("state loaded", zap.String("builder_name", b.Name), zap.String("file_name", stateFileName))
	return nil
}

func (b *Builder) loadDelta(ctx context.Context, fromBlock, exclusiveStopBlock uint64, outputCacheSaveInterval uint64, outputCacheStore dstore.Store) error {
	if b.partialMode {
		panic("cannot load a state in partial mode")
	}

	zlog.Debug("loading delta",
		zap.String("builder_name", b.Name),
		zap.Uint64("from_block", fromBlock),
		zap.Uint64("stop_block", exclusiveStopBlock),
	)

	startBlockNum := outputs.ComputeStartBlock(fromBlock, outputCacheSaveInterval)
	outputCache := outputs.NewOutputCache(b.Name, outputCacheStore, 0)

	err := outputCache.Load(ctx, startBlockNum)
	if err != nil {
		return fmt.Errorf("loading init cache for builder %q with start block %d: %w", b.Name, startBlockNum, err)
	}

	for {
		deltas := outputCache.SortedCacheItem()
		if len(deltas) == 0 {
			panic("missing deltas")
		}

		firstSeenBlockNum := uint64(0)
		lastSeenBlockNum := uint64(0)

		for _, delta := range deltas {
			//todo: we should check the from block?
			if delta.BlockNum >= exclusiveStopBlock {
				return nil //all good we reach the end
			}
			if firstSeenBlockNum == uint64(0) {
				firstSeenBlockNum = delta.BlockNum
			}
			lastSeenBlockNum = delta.BlockNum
			pbDelta := &pbsubstreams.StoreDelta{}
			err := proto.Unmarshal(delta.Payload, pbDelta)
			if err != nil {
				return fmt.Errorf("unmarshalling builder %q delta at block %d: %w", b.Name, delta.BlockNum, err)
			}
			b.Deltas = append(b.Deltas, pbDelta)
		}

		zlog.Debug("loaded deltas", zap.String("builder_name", b.Name), zap.Uint64("from_block_num", firstSeenBlockNum), zap.Uint64("to_block_num", lastSeenBlockNum))

		if exclusiveStopBlock <= outputCache.CurrentBlockRange.ExclusiveEndBlock {
			return nil
		}
		err := outputCache.Load(ctx, outputCache.CurrentBlockRange.ExclusiveEndBlock)
		if err != nil {
			return fmt.Errorf("loading more deltas: %w", err)
		}
	}
}
