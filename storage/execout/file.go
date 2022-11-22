package execout

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

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"

	"go.uber.org/zap/zapcore"

	"github.com/streamingfast/derr"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
)

// TODO(abourget): this is called File because we want it to BECOME a File, but right now it knows
// more than that.

type File struct {
	sync.RWMutex

	wg                *sync.WaitGroup
	ModuleName        string
	currentBlockRange *block.Range
	outputData        *pboutput.Map
	store             dstore.Store
	saveBlockInterval uint64
	logger            *zap.Logger

	initialized bool
}

func (c *File) currentFilename() string {
	return computeDBinFilename(c.currentBlockRange.StartBlock, c.currentBlockRange.ExclusiveEndBlock)
}

func (c *File) SortedCacheItems() (out []*pboutput.Item) {
	for _, item := range c.outputData.Kv {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BlockNum < out[j].BlockNum
	})
	return
}

func (c *File) IsInitialized() bool { return c.initialized }

func (c *File) IsOutOfRange(blockNum uint64) bool {
	if !c.initialized { // should become in-range once we Set it
		return false
	}
	return !c.currentBlockRange.Contains(blockNum)
}

func (c *File) Set(clock *pbsubstreams.Clock, cursor string, data []byte) error {
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

func (c *File) Get(clock *pbsubstreams.Clock) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()

	cacheItem, found := c.outputData.Kv[clock.Id]

	if !found {
		return nil, false
	}

	return cacheItem.Payload, found
}

func (c *File) GetAtBlock(blockNumber uint64) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()

	for _, value := range c.outputData.Kv {
		if value.BlockNum == blockNumber {
			return value.Payload, true
		}
	}

	return nil, false
}

func (c *File) LoadAtEndBlockBoundary(ctx context.Context) (found bool, err error) {
	return c.LoadAtBlock(ctx, c.currentBlockRange.ExclusiveEndBlock)
}

func (c *File) LoadAtBlock(ctx context.Context, atBlock uint64) (found bool, err error) {
	c.logger.Info("loading cache at block", zap.Uint64("at_block_num", atBlock))

	c.outputData = &pboutput.Map{
		Kv: make(map[string]*pboutput.Item),
	}

	blockRange, found, err := findBlockRange(ctx, c.store, atBlock)
	if err != nil {
		return found, fmt.Errorf("computing block range for module %q: %w", c.ModuleName, err)
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
		return false, fmt.Errorf("loading cache at %d: %w", atBlock, err)
	}

	c.initialized = true

	return found, nil

}
func (c *File) Load(ctx context.Context, blockRange *block.Range) error {
	c.logger.Debug("loading cache", zap.Object("range", blockRange))
	c.outputData.Kv = make(map[string]*pboutput.Item)

	filename := computeDBinFilename(blockRange.StartBlock, blockRange.ExclusiveEndBlock)
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

func (c *File) Save(ctx context.Context) error {
	if len(c.outputData.Kv) == 0 {
		c.logger.Info("not saving cache, because empty", zap.Stringer("block_range", c.currentBlockRange))
		return nil
	}
	// TODO(abourget): track if there are Payloads in there?
	filename := c.currentFilename()

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

func (c *File) String() string {
	return c.store.ObjectURL("")
}

func (c *File) ListContinuousCacheRanges(ctx context.Context, from uint64) (block.Ranges, error) {
	cachedRanges, err := c.ListCacheRanges(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing cached ranges %q: %w", c.ModuleName, err)
	}
	out := listContinuousCacheRanges(cachedRanges, from)
	return out, nil
}

// TODO(abourget): this doesn't belong to the "File", rather a "FileRange" or something else
func (c *File) ListCacheRanges(ctx context.Context) (block.Ranges, error) {
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

func (c *File) Delete(blockID string) {
	c.Lock()
	defer c.Unlock()

	delete(c.outputData.Kv, blockID)
}

func (c *File) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("store", c.ModuleName)
	enc.AddUint64("start_block", c.currentBlockRange.StartBlock)
	enc.AddUint64("end_block", c.currentBlockRange.ExclusiveEndBlock)
	enc.AddInt("kv_count", len(c.outputData.Kv))
	return nil
}

func (c *File) Close() {
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

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
