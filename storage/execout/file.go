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
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// A File in `execout` stores, for a given module (with a given hash), the outputs of module execution
// for _multiple blocks_, based on their block ID.
type File struct {
	sync.RWMutex
	*block.Range

	ModuleName string
	kv         map[string]*pboutput.Item
	store      dstore.Store
	logger     *zap.Logger
}

func (c *File) Filename() string {
	return computeDBinFilename(c.Range.StartBlock, c.Range.ExclusiveEndBlock)
}

func (c *File) SortedItems() (out []*pboutput.Item) {
	// TODO(abourget): eventually, what is saved should be sorted before saving,
	// or we import a list and Load() automatically sorts what needs to be sorted.
	for _, item := range c.kv {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BlockNum < out[j].BlockNum
	})
	return
}

func (c *File) ExtractClocks(clocksMap map[uint64]*pbsubstreams.Clock) {
	for _, item := range c.kv {
		if _, found := clocksMap[item.BlockNum]; !found {
			clocksMap[item.BlockNum] = &pbsubstreams.Clock{
				Number:    item.BlockNum,
				Id:        item.BlockId,
				Timestamp: item.Timestamp,
			}
		}
	}
	return
}

func (c *File) SetItem(clock *pbsubstreams.Clock, data []byte) {
	c.Lock()
	defer c.Unlock()

	cp := make([]byte, len(data))
	copy(cp, data)

	ci := &pboutput.Item{
		BlockNum:  clock.Number,
		BlockId:   clock.Id,
		Timestamp: clock.Timestamp,
		// TODO(abourget): remove the `Cursor` from this `pboutput.Item` struct,
		//  as we're only going to store irreversible stuff now.
		Payload: cp,
	}

	c.kv[clock.Id] = ci
}

func (c *File) Get(clock *pbsubstreams.Clock) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()

	cacheItem, found := c.kv[clock.Id]

	if !found {
		return nil, false
	}

	return cacheItem.Payload, found
}

func (c *File) GetAtBlock(blockNumber uint64) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()

	for _, value := range c.kv {
		if value.BlockNum == blockNumber {
			return value.Payload, true
		}
	}

	return nil, false
}

func (c *File) Load(ctx context.Context) error {
	filename := computeDBinFilename(c.Range.StartBlock, c.Range.ExclusiveEndBlock)
	c.logger.Debug("loading execout file", zap.String("file_name", filename), zap.Object("block_range", c.Range))

	return derr.RetryContext(ctx, 5, func(ctx context.Context) error {
		objectReader, err := c.store.OpenObject(ctx, filename)
		if err == dstore.ErrNotFound {
			return derr.NewFatalError(err)
		}

		if err != nil {
			return fmt.Errorf("loading block reader %s: %w", filename, err)
		}
		defer objectReader.Close()

		bytes, err := io.ReadAll(objectReader)
		if err != nil {
			return fmt.Errorf("reading store file %s: %w", filename, err)
		}

		outputData := &pboutput.Map{}
		if err = outputData.UnmarshalFast(bytes); err != nil {
			return fmt.Errorf("unmarshalling file %s: %w", filename, err)
		}

		c.kv = outputData.Kv

		c.logger.Debug("outputs data loaded", zap.Int("output_count", len(c.kv)), zap.Stringer("block_range", c.Range))
		return nil
	})
}

func (c *File) Save(ctx context.Context) error {

	filename := c.Filename()
	outputData := &pboutput.Map{Kv: c.kv}
	cnt, err := outputData.MarshalFast()
	if err != nil {
		return fmt.Errorf("unmarshalling file %s: %w", filename, err)
	}

	c.logger.Info("writing execution output file", zap.String("filename", filename))
	return derr.RetryContext(ctx, 10, func(ctx context.Context) error { // more than the usual 5 retries here because if we fail, we have to reprocess the whole segment
		reader := bytes.NewReader(cnt)
		err := c.store.WriteObject(ctx, filename, reader)
		return err
	})
}

func (c *File) String() string {
	return c.store.ObjectURL(c.Filename())
}

func (c *File) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}
	enc.AddString("module", c.ModuleName)
	enc.AddUint64("start_block", c.Range.StartBlock)
	enc.AddUint64("end_block", c.Range.ExclusiveEndBlock)
	enc.AddInt("kv_count", len(c.kv))
	return nil
}

//
//func listContinuousCacheRanges(cachedRanges block.Ranges, from uint64) block.Ranges {
//	cachedRangeCount := len(cachedRanges)
//	var out block.Ranges
//	for i, r := range cachedRanges {
//		if r.StartBlock < from {
//			continue
//		}
//		out = append(out, r)
//		if cachedRangeCount > i+1 {
//			next := cachedRanges[i+1]
//			if next.StartBlock != r.ExclusiveEndBlock { //continuous seq broken
//				break
//			}
//		}
//	}
//
//	return out
//}

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
