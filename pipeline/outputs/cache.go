package outputs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/utils"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var cacheFilenameRegex *regexp.Regexp

func init() {
	cacheFilenameRegex = regexp.MustCompile(`([\d]+)-([\d]+)\.output`)
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

func (c *ModulesOutputCache) RegisterModule(module *pbsubstreams.Module, hash string, baseCacheStore dstore.Store) (*OutputCache, error) {
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

			previousFilename := moduleCache.currentFilename()
			if err := moduleCache.save(ctx, previousFilename); err != nil {
				return fmt.Errorf("saving blocks for module kv %s: %w", moduleCache.ModuleName, err)
			}

			if _, err := moduleCache.LoadAtBlock(ctx, moduleCache.CurrentBlockRange.ExclusiveEndBlock); err != nil {
				return fmt.Errorf("loading blocks %d for module kv %s: %w", moduleCache.CurrentBlockRange.ExclusiveEndBlock, moduleCache.ModuleName, err)
			}
		}
	}

	return nil
}

func (c *ModulesOutputCache) Flush(ctx context.Context) error {
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

type CacheItem struct {
	BlockNum  uint64                 `json:"block_num"`
	BlockID   string                 `json:"block_id"`
	Payload   []byte                 `json:"payload"`
	Timestamp *timestamppb.Timestamp `json:"timestamp"`
	Cursor    string                 `json:"cursor"`
}

type outputKV map[string]*CacheItem

type OutputCache struct {
	sync.RWMutex

	ModuleName        string
	CurrentBlockRange *block.Range
	kv                outputKV
	Store             dstore.Store
	saveBlockInterval uint64
}

func NewOutputCache(moduleName string, store dstore.Store, saveBlockInterval uint64) *OutputCache {
	return &OutputCache{
		ModuleName:        moduleName,
		Store:             store,
		saveBlockInterval: saveBlockInterval,
	}
}

func (c *OutputCache) currentFilename() string {
	return computeDBinFilename(c.CurrentBlockRange.StartBlock, c.CurrentBlockRange.ExclusiveEndBlock)
}

func (c *OutputCache) SortedCacheItems() (out []*CacheItem) {
	for _, item := range c.kv {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BlockNum < out[j].BlockNum
	})
	return
}

func (c *OutputCache) IsOutOfRange(ref bstream.BlockRef) bool {
	return !c.CurrentBlockRange.ContainsBlockRef(ref)
}

func (c *OutputCache) Set(clock *pbsubstreams.Clock, cursor string, data []byte) error {
	c.Lock()
	defer c.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)

	ci := &CacheItem{
		BlockNum:  clock.Number,
		BlockID:   clock.Id,
		Timestamp: clock.Timestamp,
		Cursor:    cursor,
		Payload:   cp,
	}

	c.kv[clock.Id] = ci

	return nil
}

func (c *OutputCache) Get(clock *pbsubstreams.Clock) ([]byte, bool, error) {
	c.Lock()
	defer c.Unlock()

	cacheItem, found := c.kv[clock.Id]

	if !found {
		return nil, false, nil
	}

	return cacheItem.Payload, found, nil
}

func (c *OutputCache) LoadAtBlock(ctx context.Context, atBlock uint64) (found bool, err error) {
	zlog.Info("loading cache at block", zap.String("module_name", c.ModuleName), zap.Uint64("at_block_num", atBlock))

	c.kv = make(outputKV)

	blockRange, found, err := findBlockRange(ctx, c.Store, atBlock)
	if err != nil {
		return found, fmt.Errorf("computing block range for module %q: %w", c.ModuleName, err)
	}

	if !found {
		blockRange = block.NewRange(atBlock, atBlock+c.saveBlockInterval)
		c.CurrentBlockRange = blockRange
		return found, nil
	}

	err = c.Load(ctx, blockRange)
	if err != nil {
		return false, fmt.Errorf("loading cache: %w", err)
	}
	return found, nil

}
func (c *OutputCache) Load(ctx context.Context, blockRange *block.Range) error {
	zlog.Debug("loading cache", zap.String("module_name", c.ModuleName), zap.Object("range", blockRange))
	c.kv = make(outputKV)

	filename := computeDBinFilename(blockRange.StartBlock, blockRange.ExclusiveEndBlock)
	zlog.Debug("loading outputs data", zap.String("file_name", filename), zap.String("cache_module_name", c.ModuleName), zap.Object("block_range", blockRange))

	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		objectReader, err := c.Store.OpenObject(ctx, filename)
		if err != nil {
			return fmt.Errorf("loading block reader %s: %w", filename, err)
		}

		if err = json.NewDecoder(objectReader).Decode(&c.kv); err != nil {
			return fmt.Errorf("json decoding file %s: %w", filename, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("retried: %w", err)
	}

	c.CurrentBlockRange = blockRange
	zlog.Debug("outputs data loaded", zap.String("module_name", c.ModuleName), zap.Int("output_count", len(c.kv)), zap.Stringer("block_range", c.CurrentBlockRange))
	return nil
}

func (c *OutputCache) save(ctx context.Context, filename string) error {
	zlog.Info("saving cache", zap.String("module_name", c.ModuleName), zap.Stringer("block_range", c.CurrentBlockRange), zap.String("filename", filename))

	buffer := bytes.NewBuffer(nil)
	err := json.NewEncoder(buffer).Encode(c.kv)
	if err != nil {
		return fmt.Errorf("json encoding outputs: %w", err)
	}
	cnt := buffer.Bytes()

	go func() {
		err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
			reader := bytes.NewReader(cnt)
			return c.Store.WriteObject(ctx, filename, reader)
		})
		if err != nil {
			zlog.Warn("failed writing output cache", zap.Error(err))
		}
	}()

	return nil
}

func (c *OutputCache) String() string {
	return c.Store.ObjectURL("")
}

func (c *OutputCache) ListContinuousCacheRanges(ctx context.Context, from uint64) (block.Ranges, error) {
	cachedRanges, err := c.ListCacheRanges(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing cached ranges %q: %w", c.ModuleName, err)
	}
	out := listContinuousCacheRanges(cachedRanges, from)
	return out, nil
}

func (c *OutputCache) ListCacheRanges(ctx context.Context) (block.Ranges, error) {
	var out block.Ranges
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := c.Store.Walk(ctx, "", func(filename string) (err error) {
			r, err := fileNameToRange(filename)
			if err != nil {
				return fmt.Errorf("getting range from filename: %w", err)
			}
			out = append(out, r)
			return nil
		}); err != nil {
			return fmt.Errorf("walking cache ouputs: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartBlock < out[j].StartBlock
	})

	return out, nil
}

func (c *OutputCache) Delete(blockID string) {
	c.Lock()
	defer c.Unlock()

	delete(c.kv, blockID)
}

func listContinuousCacheRanges(cachedRanges block.Ranges, from uint64) block.Ranges {
	cachedRangeCount := len(cachedRanges)
	var out block.Ranges
	for i, r := range cachedRanges {
		if r.StartBlock < from {
			continue
		}
		out = append(out, r)
		if cachedRangeCount > i+1 {
			next := cachedRanges[i+1]
			if next.StartBlock != r.ExclusiveEndBlock { //continuous seq broken
				break
			}
		}
	}

	return out
}

func fileNameToRange(filename string) (*block.Range, error) {
	res := cacheFilenameRegex.FindAllStringSubmatch(filename, 1)
	if len(res) != 1 {
		return nil, fmt.Errorf("invalid output cache filename, %q", filename)
	}

	start := uint64(utils.MustAtoi(res[0][1]))
	end := uint64(utils.MustAtoi(res[0][2]))

	return &block.Range{
		StartBlock:        start,
		ExclusiveEndBlock: end,
	}, nil
}

func findBlockRange(ctx context.Context, store dstore.Store, prefixStartBlock uint64) (*block.Range, bool, error) {
	var exclusiveEndBlock uint64

	paddedBlock := pad(prefixStartBlock)

	var files []string
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) (err error) {
		files, err = store.ListFiles(ctx, paddedBlock, math.MaxInt64)
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

	return block.NewRange(prefixStartBlock, exclusiveEndBlock), true, nil
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
