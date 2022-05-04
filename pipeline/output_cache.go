package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	pbbstream "github.com/streamingfast/pbgo/sf/bstream/v1"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type outputCache struct {
	moduleName        string
	currentBlockRange *blockRange
	kv                map[string]*bstream.Block
	store             dstore.Store
	new               bool
}

type ModulesOutputCache struct {
	outputCaches map[string]*outputCache
	lock         sync.RWMutex
}

type blockRange struct {
	startBlock        uint64
	exclusiveEndBlock uint64
}

func NewModuleOutputCache(ctx context.Context, modules []*pbsubstreams.Module, manif *pbsubstreams.Manifest, graph *manifest.ModuleGraph, baseOutputCacheStore dstore.Store) (*ModulesOutputCache, error) {
	moduleOutputCache := &ModulesOutputCache{
		outputCaches: make(map[string]*outputCache),
	}

	for _, module := range modules {
		hash := manifest.HashModuleAsString(manif, graph, module)

		moduleStore, err := baseOutputCacheStore.SubStore(fmt.Sprintf("%s-%s/outputs", module.Name, hash))
		if err != nil {
			return nil, fmt.Errorf("creating substore for module %q: %w", module.Name, err)
		}

		cache := &outputCache{
			moduleName: module.Name,
			store:      moduleStore,
		}

		moduleOutputCache.outputCaches[module.Name] = cache

		if err = cache.loadBlocks(ctx, module.StartBlock); err != nil {
			return nil, fmt.Errorf("loading blocks for module %q: %w", module.Name, err)
		}
	}

	return moduleOutputCache, nil
}

func (c *ModulesOutputCache) update(ctx context.Context, blockRef bstream.BlockRef) error {
	for _, moduleCache := range c.outputCaches {
		if !moduleCache.currentBlockRange.contains(blockRef) {
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

func (c *ModulesOutputCache) set(moduleName string, block *bstream.Block, data []byte) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	cache := c.outputCaches[moduleName]

	if !cache.new {
		zlog.Warn("trying to add output to an already existing module kv", zap.String("module_name", moduleName))
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

	cache.kv[block.Id] = pbBlock

	return nil
}

func (c *ModulesOutputCache) get(moduleName string, block *bstream.Block) ([]byte, bool, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	cache := c.outputCaches[moduleName]

	b, found := cache.kv[block.Id]

	if !found {
		return nil, false, nil
	}

	data, err := b.Payload.Get()

	return data, found, err

}

func (r *blockRange) contains(blockRef bstream.BlockRef) bool {
	return blockRef.Num() >= r.startBlock && blockRef.Num() < r.exclusiveEndBlock
}

func (o *outputCache) loadBlocks(ctx context.Context, startBlock uint64) (err error) {
	var found bool

	o.new = false
	o.kv = make(map[string]*bstream.Block)
	o.currentBlockRange = nil

	o.currentBlockRange, found, err = findBlockRange(ctx, o.store, startBlock)
	if err != nil {
		return fmt.Errorf("computing block range for module %q: %w", o.moduleName, err)
	}

	if !found {
		o.new = true
		return nil
	}

	filename := computeDBinFilename(pad(o.currentBlockRange.startBlock), pad(o.currentBlockRange.exclusiveEndBlock))
	objectReader, err := o.store.OpenObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("loading block reader %s: %w", filename, err)
	}

	blockReader, err := bstream.GetBlockReaderFactory.New(objectReader)
	if err != nil {
		return fmt.Errorf("getting block reader %s: %w", filename, err)
	}

	for {
		block, err := blockReader.Read()

		if err != io.EOF {
			return fmt.Errorf("reading block: %w", err)
		}

		if block == nil {
			return nil
		}

		o.kv[block.Id] = block

		if err == io.EOF {
			return nil
		}
	}
}

func (o *outputCache) saveBlocks(ctx context.Context) error {
	filename := computeDBinFilename(pad(o.currentBlockRange.startBlock), pad(o.currentBlockRange.exclusiveEndBlock))

	buffer := bytes.NewBuffer(nil)
	blockWriter, err := bstream.GetBlockWriterFactory.New(buffer)
	if err != nil {
		return fmt.Errorf("write block factory: %w", err)
	}

	for _, block := range o.kv {
		if err := blockWriter.Write(block); err != nil {
			return fmt.Errorf("write block: %w", err)
		}
	}

	err = o.store.WriteObject(ctx, filename, buffer)
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
