package outputs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

type CacheItem struct {
	BlockNum uint64 `json:"block_num"`
	BlockID  string
	Payload  []byte `json:"payload"`
}
type outputKV map[string]*CacheItem
type OutputCache struct {
	lock sync.RWMutex

	ModuleName        string
	CurrentBlockRange *block.Range
	//kv                map[string]*bstream.Block
	kv                outputKV
	Store             dstore.Store
	saveBlockInterval uint64
	//Completed         bool
}

func (c *OutputCache) currentFilename() string {
	return computeDBinFilename(c.CurrentBlockRange.StartBlock, c.CurrentBlockRange.ExclusiveEndBlock)
}

type ModulesOutputCache struct {
	OutputCaches      map[string]*OutputCache
	SaveBlockInterval uint64
}

func NewModuleOutputCache(saveBlockInterval uint64) *ModulesOutputCache {
	zlog.Debug("creating cache with modules")
	moduleOutputCache := &ModulesOutputCache{
		OutputCaches:      make(map[string]*OutputCache),
		SaveBlockInterval: saveBlockInterval,
	}

	return moduleOutputCache
}

func (c *ModulesOutputCache) RegisterModule(ctx context.Context, module *pbsubstreams.Module, hash string, baseCacheStore dstore.Store, requestedStartBlock uint64) (*OutputCache, error) {
	zlog.Debug("registering modules", zap.String("module_name", module.Name))

	if cache, found := c.OutputCaches[module.Name]; found {
		return cache, nil
	}

	moduleStore, err := baseCacheStore.SubStore(fmt.Sprintf("%s/outputs", hash))
	if err != nil {
		return nil, fmt.Errorf("creating substore for module %q: %w", module.Name, err)
	}

	cache := NewOutputCache(module.Name, moduleStore, c.SaveBlockInterval)

	c.OutputCaches[module.Name] = cache

	return cache, nil
}

func (c *ModulesOutputCache) Update(ctx context.Context, blockRef bstream.BlockRef) error {
	for _, moduleCache := range c.OutputCaches {
		if moduleCache.IsOutOfRange(blockRef) {
			zlog.Debug("updating cache", zap.Stringer("block_ref", blockRef))
			//this is a complete range
			previousFilename := moduleCache.currentFilename()
			if err := moduleCache.save(ctx, previousFilename); err != nil {
				return fmt.Errorf("saving blocks for module kv %s: %w", moduleCache.ModuleName, err)
			}

			if _, err := moduleCache.Load(ctx, moduleCache.CurrentBlockRange.ExclusiveEndBlock); err != nil {
				return fmt.Errorf("loading blocks %d for module kv %s: %w", moduleCache.CurrentBlockRange.ExclusiveEndBlock, moduleCache.ModuleName, err)
			}
		}
	}

	return nil
}

func (c *ModulesOutputCache) Save(ctx context.Context) error {
	zlog.Info("Saving caches")
	for _, moduleCache := range c.OutputCaches {
		filename := moduleCache.currentFilename()
		zlog.Debug("saving cache for current block range", zap.String("module_name", moduleCache.ModuleName),
			zap.Uint64("start_block", moduleCache.CurrentBlockRange.StartBlock),
			zap.Uint64("end_block", moduleCache.CurrentBlockRange.ExclusiveEndBlock))

		if err := moduleCache.save(ctx, filename); err != nil {
			return fmt.Errorf("save: saving outpust or module kv %s: %w", moduleCache.ModuleName, err)
		}
	}
	return nil
}

func NewOutputCache(moduleName string, store dstore.Store, saveBlockInterval uint64) *OutputCache {
	return &OutputCache{
		ModuleName:        moduleName,
		Store:             store,
		saveBlockInterval: saveBlockInterval,
	}
}

func (o *OutputCache) SortedCacheItems() (out []*CacheItem) {
	for _, item := range o.kv {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BlockNum < out[j].BlockNum
	})
	return
}

func (o *OutputCache) IsOutOfRange(ref bstream.BlockRef) bool {
	return !o.CurrentBlockRange.Contains(ref)
}

func (o *OutputCache) Set(block *bstream.Block, data []byte) error {
	o.lock.Lock()
	defer o.lock.Unlock()

	ci := &CacheItem{
		BlockNum: block.Num(),
		BlockID:  block.Id,
		Payload:  data,
	}

	o.kv[block.Id] = ci

	return nil
}

func (o *OutputCache) Get(block *bstream.Block) ([]byte, bool, error) {
	o.lock.Lock()
	defer o.lock.Unlock()

	cacheItem, found := o.kv[block.Id]

	if !found {
		return nil, false, nil
	}

	return cacheItem.Payload, found, nil
}

func (o *OutputCache) Load(ctx context.Context, atBlock uint64) (foud bool, err error) {
	zlog.Info("loading outputs", zap.String("module_name", o.ModuleName), zap.Uint64("at_block_num", atBlock))

	o.kv = make(outputKV)

	var found bool
	o.CurrentBlockRange, found, err = findBlockRange(ctx, o.Store, atBlock)
	if err != nil {
		return found, fmt.Errorf("computing block range for module %q: %w", o.ModuleName, err)
	}

	if !found {
		o.CurrentBlockRange = &block.Range{
			StartBlock:        atBlock,
			ExclusiveEndBlock: atBlock + o.saveBlockInterval,
		}

		return found, nil
	}

	filename := computeDBinFilename(o.CurrentBlockRange.StartBlock, o.CurrentBlockRange.ExclusiveEndBlock)
	zlog.Debug("loading outputs data", zap.String("file_name", filename), zap.String("cache_module_name", o.ModuleName), zap.Object("block_range", o.CurrentBlockRange))

	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		objectReader, err := o.Store.OpenObject(ctx, filename)
		if err != nil {
			return fmt.Errorf("loading block reader %s: %w", filename, err)
		}

		if err = json.NewDecoder(objectReader).Decode(&o.kv); err != nil {
			return fmt.Errorf("json decoding file %s: %w", filename, err)
		}

		return nil
	})
	if err != nil {
		return false, fmt.Errorf("retried: %w", err)
	}

	zlog.Debug("outputs data loaded", zap.String("module_name", o.ModuleName), zap.Int("output_count", len(o.kv)), zap.Stringer("block_range", o.CurrentBlockRange))
	return found, nil
}

func (o *OutputCache) save(ctx context.Context, filename string) error {
	zlog.Info("saving cache", zap.String("module_name", o.ModuleName), zap.Stringer("block_range", o.CurrentBlockRange), zap.String("filename", filename))

	buffer := bytes.NewBuffer(nil)
	err := json.NewEncoder(buffer).Encode(o.kv)
	if err != nil {
		return fmt.Errorf("json encoding outputs: %w", err)
	}
	cnt := buffer.Bytes()

	go func() {
		err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
			reader := bytes.NewReader(cnt)
			return o.Store.WriteObject(ctx, filename, reader)
		})
		if err != nil {
			zlog.Warn("failed writing output cache", zap.Error(err))
		}
	}()

	return nil
}

func (o *OutputCache) String() string {
	return o.Store.ObjectURL("")
}

func findBlockRange(ctx context.Context, store dstore.Store, prefixStartBlock uint64) (*block.Range, bool, error) {
	var exclusiveEndBlock uint64

	paddedBlock := pad(prefixStartBlock)

	var files []string
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) (err error) {
		files, err = store.ListFiles(ctx, paddedBlock, ".tmp", math.MaxInt64)
		return
	})
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

	return &block.Range{
		StartBlock:        prefixStartBlock,
		ExclusiveEndBlock: exclusiveEndBlock,
	}, true, nil
}

func computeDBinFilename(startBlock, stopBlock uint64) string {
	return fmt.Sprintf("%010d-%010d.output", startBlock, stopBlock)
}

func pad(blockNumber uint64) string {
	return fmt.Sprintf("%010d", blockNumber)
}

func ComputeStartBlock(startBlock uint64, saveBlockInterval uint64) uint64 {
	return startBlock - startBlock%saveBlockInterval
}

func getExclusiveEndBlock(filename string) (uint64, error) {
	endBlock := strings.Split(strings.Split(filename, "-")[1], ".")[0]
	parsedInt, err := strconv.ParseInt(strings.TrimLeft(endBlock, "0"), 10, 64)

	if err != nil {
		return 0, fmt.Errorf("parsing int %d: %w", parsedInt, err)
	}

	return uint64(parsedInt), nil
}
