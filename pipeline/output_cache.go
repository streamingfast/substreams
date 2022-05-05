package pipeline

import (
	"bytes"
	"context"
	"fmt"
	pbbstream "github.com/streamingfast/pbgo/sf/bstream/v1"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type outputCache struct {
	lock sync.RWMutex

	moduleName        string
	currentBlockRange *blockRange
	kv                map[string]*bstream.Block
	store             dstore.Store
	new               bool
}

type ModulesOutputCache struct {
	outputCaches map[string]*outputCache
}

func NewModuleOutputCache() *ModulesOutputCache {
	zlog.Debug("creating cache with modules")
	moduleOutputCache := &ModulesOutputCache{
		outputCaches: make(map[string]*outputCache),
	}

	return moduleOutputCache
}

func (c *ModulesOutputCache) registerModule(ctx context.Context, module *pbsubstreams.Module, hash string, baseOutputCacheStore dstore.Store, requestedStartBlock uint64) (*outputCache, error) {
	zlog.Debug("modules", zap.String("module_name", module.Name))

	moduleStore, err := baseOutputCacheStore.SubStore(fmt.Sprintf("%s-%s/outputs", module.Name, hash))
	if err != nil {
		return nil, fmt.Errorf("creating substore for module %q: %w", module.Name, err)
	}

	cache := &outputCache{
		moduleName: module.Name,
		store:      moduleStore,
	}

	c.outputCaches[module.Name] = cache

	if err = cache.loadBlocks(ctx, computeStartBlock(requestedStartBlock)); err != nil {
		return nil, fmt.Errorf("loading blocks for module %q: %w", module.Name, err)
	}

	return cache, nil
}

func (c *ModulesOutputCache) update(ctx context.Context, blockRef bstream.BlockRef) error {
	for _, moduleCache := range c.outputCaches {
		if !moduleCache.currentBlockRange.contains(blockRef) {
			zlog.Debug("updating cache", zap.Stringer("block_ref", blockRef))
			if err := moduleCache.saveBlocks(ctx); err != nil {
				return fmt.Errorf("saving blocks for module kv %s: %w", moduleCache.moduleName, err)
			}
			if err := moduleCache.loadBlocks(ctx, moduleCache.currentBlockRange.exclusiveEndBlock); err != nil {
				return fmt.Errorf("loading blocks for module kv %s: %w", moduleCache.moduleName, err)
			}
		}
	}

	return nil
}

func (r *blockRange) contains(blockRef bstream.BlockRef) bool {
	return blockRef.Num() >= r.startBlock && blockRef.Num() < r.exclusiveEndBlock
}

func (c *outputCache) set(block *bstream.Block, data []byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if !c.new {
		zlog.Warn("trying to add output to an already existing module kv", zap.String("module_name", c.moduleName))
		return nil
	}

	pbBlock := &bstream.Block{
		Id:             block.ID(),
		Number:         block.Num(),
		PreviousId:     block.PreviousID(),
		Timestamp:      block.Time(),
		LibNum:         block.LIBNum(),
		PayloadKind:    pbbstream.Protocol_UNKNOWN,
		PayloadVersion: int32(1),
	}

	_, err := bstream.GetBlockPayloadSetter(pbBlock, data)
	if err != nil {
		return fmt.Errorf("setting block payload for block %s: %w", block.Id, err)
	}

	c.kv[block.Id] = pbBlock

	return nil
}

func (c *outputCache) get(block *bstream.Block) ([]byte, bool, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	b, found := c.kv[block.Id]

	if !found {
		return nil, false, nil
	}

	data, err := b.Payload.Get()

	return data, found, err

}

func (c *outputCache) loadBlocks(ctx context.Context, atBlock uint64) (err error) {
	var found bool

	c.new = false
	c.kv = make(map[string]*bstream.Block)

	c.currentBlockRange, found, err = findBlockRange(ctx, c.store, atBlock)
	zlog.Info("loading blocks", zap.Stringer("block_range", c.currentBlockRange))
	if err != nil {
		return fmt.Errorf("computing block range for module %q: %w", c.moduleName, err)
	}

	if !found {
		c.currentBlockRange = &blockRange{
			startBlock:        atBlock,
			exclusiveEndBlock: atBlock + 100,
		}

		c.new = true
		return nil
	}

	filename := computeDBinFilename(pad(c.currentBlockRange.startBlock), pad(c.currentBlockRange.exclusiveEndBlock))
	objectReader, err := c.store.OpenObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("loading block reader %s: %w", filename, err)
	}

	blockReader, err := bstream.GetBlockReaderFactory.New(objectReader)
	if err != nil {
		return fmt.Errorf("getting block reader %s: %w", filename, err)
	}

	for {
		block, err := blockReader.Read()

		if err != nil && err != io.EOF {
			return fmt.Errorf("reading block: %w", err)
		}

		if block == nil {
			return nil
		}

		c.kv[block.Id] = block

		if err == io.EOF {
			return nil
		}
	}
}

func (c *outputCache) saveBlocks(ctx context.Context) error {
	zlog.Info("saving cache", zap.String("module_name", c.moduleName), zap.Stringer("block_range", c.currentBlockRange))
	filename := computeDBinFilename(pad(c.currentBlockRange.startBlock), pad(c.currentBlockRange.exclusiveEndBlock))

	buffer := bytes.NewBuffer(nil)
	blockWriter, err := bstream.GetBlockWriterFactory.New(buffer)
	if err != nil {
		return fmt.Errorf("write block factory: %w", err)
	}

	for _, block := range c.kv {
		if err := blockWriter.Write(block); err != nil {
			return fmt.Errorf("write block: %w", err)
		}
	}

	err = c.store.WriteObject(ctx, filename, buffer)
	if err != nil {
		return fmt.Errorf("writing block buffer to store: %w", err)
	}

	return nil
}

func findBlockRange(ctx context.Context, store dstore.Store, prefixStartBlock uint64) (*blockRange, bool, error) {
	var exclusiveEndBlock uint64

	paddedBlock := pad(prefixStartBlock)

	files, err := store.ListFiles(ctx, paddedBlock, ".tmp", math.MaxInt64)
	if err != nil {
		return nil, false, fmt.Errorf("walking prefix for padded block %s: %w", paddedBlock, err)
	}

	if len(files) == 0 {
		return nil, false, nil
	}

	biggestEndBlock := uint64(0)

	for _, file := range files {
		endBlock, err := getExclusiveEndBlock(file)
		if err != nil {
			return nil, false, fmt.Errorf("getting exclusive end block from file %s: %w", file, err)
		}
		if endBlock > biggestEndBlock {
			biggestEndBlock = endBlock
		}
	}

	exclusiveEndBlock = biggestEndBlock

	return &blockRange{
		startBlock:        prefixStartBlock,
		exclusiveEndBlock: exclusiveEndBlock,
	}, true, nil
}

func computeDBinFilename(startBlock, stopBlock string) string {
	return fmt.Sprintf("%s-%s", startBlock, stopBlock)
}

func pad(blockNumber uint64) string {
	return fmt.Sprintf("000%d", blockNumber)
}

func computeStartBlock(startBlock uint64) uint64 {
	blockInterval := uint64(100) // fixme: get from firehose flag?
	return startBlock - startBlock%blockInterval
}

func getExclusiveEndBlock(filename string) (uint64, error) {
	endBlock := strings.Split(filename, "-")[1]
	parsedInt, err := strconv.ParseInt(strings.TrimPrefix(strings.Split(endBlock, ".")[0], "000"), 10, 64)

	if err != nil {
		return 0, fmt.Errorf("parsing int %d: %w", parsedInt, err)
	}

	return uint64(parsedInt), nil
}

type blockRange struct {
	startBlock        uint64
	exclusiveEndBlock uint64
}

func (r *blockRange) String() string {
	return fmt.Sprintf("start: %d exclusiveEndBlock: %d", r.startBlock, r.exclusiveEndBlock)
}
