package cachev1

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap/zapcore"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pboutput "github.com/streamingfast/substreams/pipeline/execout/cachev1/pb"

	"go.uber.org/zap"
)

// TODO(abourget): rename to `Item` ?
type OutputCache struct {
	sync.RWMutex

	wg                *sync.WaitGroup
	moduleName        string
	currentBlockRange *block.Range
	outputData        *pboutput.Map
	store             dstore.Store
	saveBlockInterval uint64
	logger            *zap.Logger

	initialized bool
}

// TODO(abourget): rename to Open
func NewOutputCache(moduleName string, store dstore.Store, saveBlockInterval uint64, logger *zap.Logger) *OutputCache {
	return &OutputCache{
		wg:                &sync.WaitGroup{},
		moduleName:        moduleName,
		store:             store,
		saveBlockInterval: saveBlockInterval,
		logger:            logger.Named("cache").With(zap.String("module_name", moduleName)),
	}
}

func (c *OutputCache) currentFilename() string {
	return ComputeDBinFilename(c.currentBlockRange.StartBlock, c.currentBlockRange.ExclusiveEndBlock)
}

func (c *OutputCache) SortedCacheItems() (out []*pboutput.Item) {
	for _, item := range c.outputData.Kv {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BlockNum < out[j].BlockNum
	})
	return
}

func (c *OutputCache) isOutOfRange(blockNum uint64) bool {
	if !c.initialized { // should become in-range once we Set it
		return false
	}
	return !c.currentBlockRange.Contains(blockNum)
}

//func (c *OutputCache) IsAtUpperBoundary(ref bstream.BlockRef) bool {
//	incRef := bstream.NewBlockRef(ref.ID(), ref.Num()+1)
//	return c.isOutOfRange(incRef)
//}

func (c *OutputCache) Set(clock *pbsubstreams.Clock, cursor string, data []byte) error {
	c.Lock()
	defer c.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)

	ci := &pboutput.Item{
		BlockNum:  clock.Number,
		BlockId:   clock.Id,
		Timestamp: clock.Timestamp,
		Cursor:    cursor,
		Payload:   cp,
	}

	c.outputData.Kv[clock.Id] = ci

	return nil
}

func (c *OutputCache) Get(clock *pbsubstreams.Clock) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()

	cacheItem, found := c.outputData.Kv[clock.Id]

	if !found {
		return nil, false
	}

	return cacheItem.Payload, found
}

func (c *OutputCache) GetAtBlock(blockNumber uint64) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()

	for _, value := range c.outputData.Kv {
		if value.BlockNum == blockNumber {
			return value.Payload, true
		}
	}

	return nil, false
}

func (c *OutputCache) LoadAtBlock(ctx context.Context, atBlock uint64) (found bool, err error) {
	c.logger.Info("loading cache at block", zap.Uint64("at_block_num", atBlock))

	c.outputData = &pboutput.Map{
		Kv: make(map[string]*pboutput.Item),
	}

	blockRange, found, err := findBlockRange(ctx, c.store, atBlock)
	if err != nil {
		return found, fmt.Errorf("computing block range for module %q: %w", c.moduleName, err)
	}

	c.logger.Debug("block range found", zap.Object("block_range", blockRange))

	if !found {
		endBlockRange := (atBlock - (atBlock % c.saveBlockInterval)) + c.saveBlockInterval
		blockRange = block.NewRange(atBlock, endBlockRange)
		c.currentBlockRange = blockRange
		return found, nil
	}

	err = c.Load(ctx, blockRange)
	if err != nil {
		return false, fmt.Errorf("loading cache: %w", err)
	}
	return found, nil

}
func (c *OutputCache) Load(ctx context.Context, blockRange *block.Range) error {
	c.logger.Debug("loading cache", zap.Object("range", blockRange))
	c.outputData.Kv = make(map[string]*pboutput.Item)

	filename := ComputeDBinFilename(blockRange.StartBlock, blockRange.ExclusiveEndBlock)
	c.logger.Debug("loading outputs data", zap.String("file_name", filename), zap.Object("block_range", blockRange))

	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		objectReader, err := c.store.OpenObject(ctx, filename)
		if err != nil {
			return fmt.Errorf("loading block reader %s: %w", filename, err)
		}
		defer objectReader.Close()

		bytes, err := io.ReadAll(objectReader)
		if err != nil {
			return fmt.Errorf("reading store file %s: %w", filename, err)
		}

		if err = c.outputData.UnmarshalFast(bytes); err != nil {
			return fmt.Errorf("unmarshalling file %s: %w", filename, err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("retried: %w", err)
	}

	c.currentBlockRange = blockRange
	c.logger.Debug("outputs data loaded", zap.Int("output_count", len(c.outputData.Kv)), zap.Stringer("block_range", c.currentBlockRange))
	return nil
}

func (c *OutputCache) save(ctx context.Context, filename string) error {
	c.logger.Info("saving cache", zap.Stringer("block_range", c.currentBlockRange), zap.String("filename", filename))

	cnt, err := c.outputData.MarshalFast()
	if err != nil {
		return fmt.Errorf("unmarshalling file %s: %w", filename, err)
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
			reader := bytes.NewReader(cnt)
			err := c.store.WriteObject(ctx, filename, reader)
			return err
		})
		if err != nil {
			c.logger.Warn("failed writing output cache", zap.Error(err))
		}
	}()

	return nil
}

func (c *OutputCache) String() string {
	return c.store.ObjectURL("")
}

func (c *OutputCache) ListContinuousCacheRanges(ctx context.Context, from uint64) (block.Ranges, error) {
	cachedRanges, err := c.ListCacheRanges(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing cached ranges %q: %w", c.moduleName, err)
	}
	out := listContinuousCacheRanges(cachedRanges, from)
	return out, nil
}

func (c *OutputCache) ListCacheRanges(ctx context.Context) (block.Ranges, error) {
	var out block.Ranges
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := c.store.Walk(ctx, "", func(filename string) (err error) {
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

	delete(c.outputData.Kv, blockID)
}

func (c *OutputCache) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("store", c.moduleName)
	enc.AddUint64("start_block", c.currentBlockRange.StartBlock)
	enc.AddUint64("end_block", c.currentBlockRange.ExclusiveEndBlock)
	return nil
}

func (c *OutputCache) Close() {
	c.wg.Wait()
	return
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

	start := uint64(mustAtoi(res[0][1]))
	end := uint64(mustAtoi(res[0][2]))

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

func ComputeDBinFilename(startBlock, stopBlock uint64) string {
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

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
