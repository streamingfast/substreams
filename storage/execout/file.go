package execout

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"sync"

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"github.com/streamingfast/substreams/storage/execout/streamproto"

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
	Kv         map[string]*pboutput.Item
	store      dstore.Store
	logger     *zap.Logger
	loaded     bool
	loadedSize uint64

	writeAsYouGo  bool
	writingFile   *io.PipeWriter
	writeError    chan error
	deletedBefore *uint64
}

func (c *File) FullFilename() string {
	return path.Join(c.store.BaseURL().String(), c.Filename())
}
func (c *File) Filename() string {
	return computeDBinFilename(c.Range.StartBlock, c.Range.ExclusiveEndBlock)
}

func (c *File) SortedItems() (out []*pboutput.Item) {
	// TODO(abourget): eventually, what is saved should be sorted before saving,
	// or we import a list and Load() automatically sorts what needs to be sorted.
	for _, item := range c.Kv {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BlockNum < out[j].BlockNum
	})
	return
}

func (c *File) ExtractClocks(clocksMap map[uint64]*pbsubstreams.Clock) {
	for _, item := range c.Kv {
		if _, found := clocksMap[item.BlockNum]; !found {
			clocksMap[item.BlockNum] = &pbsubstreams.Clock{
				Number:    item.BlockNum,
				Id:        item.BlockId,
				Timestamp: item.Timestamp,
			}
		}
	}
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

	if c.writeAsYouGo {
		c.writePreviousKV()
		c.Kv = make(map[string]*pboutput.Item)
		c.deletedBefore = &clock.Number
	}
	c.Kv[clock.Id] = ci

}

func (c *File) writePreviousKV() {
	for _, item := range c.Kv {
		if err := streamproto.WriteItem(c.writingFile, item); err != nil {
			c.writingFile.CloseWithError(err)
			return
		}
	}
}

func (c *File) Get(clock *pbsubstreams.Clock) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()
	if c.deletedBefore != nil && clock.Number < *c.deletedBefore {
		panic("trying to get deleted data, this should never happen")
	}

	cacheItem, found := c.Kv[clock.Id]

	if !found {
		return nil, false
	}

	return cacheItem.Payload, found
}

func (c *File) GetAtBlock(blockNumber uint64) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()

	if c.deletedBefore != nil && blockNumber < *c.deletedBefore {
		panic("trying to get deleted data, this should never happen")
	}
	for _, value := range c.Kv {
		if value.BlockNum == blockNumber {
			return value.Payload, true
		}
	}

	return nil, false
}

func (c *File) Load(ctx context.Context) error {
	c.Lock()
	defer c.Unlock()
	if c.loaded {
		return nil
	}

	filename := computeDBinFilename(c.Range.StartBlock, c.Range.ExclusiveEndBlock)
	c.logger.Debug("loading execout file", zap.String("file_name", filename), zap.Object("block_range", c.Range))

	err := derr.RetryContext(ctx, 5, func(ctx context.Context) error {
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
		c.loadedSize = uint64(len(bytes))

		outputData := &pboutput.Map{}
		if err = outputData.UnmarshalFast(bytes); err != nil {
			return fmt.Errorf("unmarshalling file %s: %w", filename, err)
		}

		c.Kv = outputData.Kv

		c.logger.Debug("outputs data loaded", zap.Int("output_count", len(c.Kv)), zap.Stringer("block_range", c.Range))
		return nil
	})
	if err == nil {
		c.loaded = true
	}
	return err
}

func (c *File) Save(ctx context.Context) error {
	if c.writeAsYouGo {
		c.writePreviousKV() // last block output must be written
		c.writingFile.Close()
		return <-c.writeError
	}

	filename := c.Filename()

	c.logger.Info("writing execution output file", zap.String("filename", filename))
	return derr.RetryContext(ctx, 10, func(ctx context.Context) error { // more than the usual 5 retries here because if we fail, we have to reprocess the whole segment
		r, w := io.Pipe()
		go func() {
			for _, item := range c.Kv {
				if err := streamproto.WriteItem(w, item); err != nil {
					w.CloseWithError(err)
					return
				}
			}
			w.Close()
		}()
		err := c.store.WriteObject(ctx, filename, r)
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
	enc.AddInt("kv_count", len(c.Kv))
	return nil
}

func computeDBinFilename(startBlock, stopBlock uint64) string {
	return fmt.Sprintf("%010d-%010d.output", startBlock, stopBlock)
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
