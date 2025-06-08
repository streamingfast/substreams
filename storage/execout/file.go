package execout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"sort"
	"strconv"
	"sync"

	"connectrpc.com/connect"
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

	readingFile io.ReadCloser
	loadedUpTo  uint64

	writingFile        *io.PipeWriter
	orderedFlagWritten bool
	writeError         chan error
	deletedBefore      *uint64

	sizeInMemory int
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

func (c *File) SetItem(clock *pbsubstreams.Clock, data []byte) error {
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

	if c.writingFile != nil {
		// if we are writing the file, we flush the data and delete what was there before.
		// if we are not writing the file, we probably writing an index so we can't delete the data, it will be aggregated at the end of the segment
		if err := c.writePreviousKV(); err != nil {
			return err
		}
		c.Kv = make(map[string]*pboutput.Item) // in writable File, we delete previous items
		c.deletedBefore = &clock.Number
	}

	c.Kv[clock.Id] = ci

	return nil
}

func (c *File) writePreviousKV() error {
	if !c.orderedFlagWritten {
		if _, err := streamproto.WriteOrderedBool(c.writingFile); err != nil {
			return fmt.Errorf("writing ordered bool: %w", err)
		}
	}
	for _, item := range c.Kv {
		size, err := streamproto.WriteItem(c.writingFile, item)
		if err != nil {
			return err
		}
		c.sizeInMemory += size
		if c.sizeInMemory > MaxExecoutSegmentSize {
			return fmt.Errorf("%w: file %s on module %s", ErrSegmentSizeExceeded, c.Filename(), c.ModuleName)
		}
	}
	return nil
}

func (c *File) Get(ctx context.Context, clock *pbsubstreams.Clock) ([]byte, bool) {
	c.Lock()
	defer c.Unlock()
	if c.deletedBefore != nil && clock.Number < *c.deletedBefore {
		panic("trying to get deleted data, this should never happen")
	}

	if c.readingFile != nil && clock.Number > c.loadedUpTo {
		c.LoadUpTo(ctx, clock.Number)
	}

	cacheItem, found := c.Kv[clock.Id]

	if !found {
		return nil, false
	}

	return cacheItem.Payload, found
}

func (c *File) GetAtBlock(ctx context.Context, blockNumber uint64) (payload []byte, found bool, err error) {
	c.Lock()
	defer c.Unlock()

	if c.deletedBefore != nil && blockNumber < *c.deletedBefore {
		panic("trying to get deleted data, this should never happen")
	}

	if err := c.LoadUpTo(ctx, blockNumber); err != nil {
		return nil, false, err
	}

	for _, value := range c.Kv {
		if value.BlockNum == blockNumber {
			return value.Payload, true, nil
		}
	}

	return nil, false, nil
}

var MaxExecoutSegmentSize = int(8589934592)
var ErrSegmentSizeExceeded = connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("execution output segment size exceeded %d bytes: substreams cannot process this segment", MaxExecoutSegmentSize))

func (c *File) LoadUpTo(ctx context.Context, upTo uint64) error {
	for upTo < c.loadedUpTo {
		_, err := c.LoadNext(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *File) openFileToRead(ctx context.Context) error {
	r, err := c.store.OpenObject(ctx, c.Filename())
	if err != nil {
		return err
	}
	ordered, readBytes, err := streamproto.ReadOrderedBool(r)
	if err != nil {
		return err
	}
	if !ordered {
		if err := rewriteAsOrdered(ctx, r, readBytes, c.store, c.Filename(), c.Range, c.logger); err != nil {
			return err
		}
		if err := r.Close(); err != nil {
			return err
		}

		r, err := c.store.OpenObject(ctx, c.Filename())
		if err != nil {
			return err
		}
		ordered, readBytes, err = streamproto.ReadOrderedBool(r)
		if err != nil {
			return err
		}
		if !ordered {
			return fmt.Errorf("internal error: could not rewrite outputs as ordered")
		}
	}

	c.readingFile = r
	return nil
}

func (c *File) LoadNext(ctx context.Context) (*pboutput.Item, error) {
	c.Lock()
	defer c.Unlock()
	if c.loadedUpTo == math.MaxUint64 {
		return nil, nil
	}

	if c.readingFile == nil {
		if err := c.openFileToRead(ctx); err != nil {
			return nil, err
		}
	}

	item, err := streamproto.ReadNextItem(c.readingFile)
	if err != nil {
		return nil, err
	}
	c.Kv[item.BlockId] = item
	c.loadedUpTo = item.BlockNum

	return item, nil
}

func rewriteAsOrdered(ctx context.Context, r io.Reader, readBytes []byte, store dstore.Store, filename string, rng *block.Range, logger *zap.Logger) error {
	bytes, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading store file %s: %w", filename, err)
	}
	bytes = append(readBytes, bytes...)

	o := &pboutput.Array{}
	if err := o.UnmarshalVTUnsafe(bytes); err != nil {
		return fmt.Errorf("unmarshalling data: %w", err)
	}
	items := o.Items

	sort.Slice(items, func(i, j int) bool {
		return items[i].BlockNum < items[j].BlockNum
	})

	newFile := &File{
		store:  store,
		logger: logger,
		Range:  rng,
	}
	newFile.WriteAsYouGo(ctx)

	for _, item := range items {
		if err := newFile.SetItem(&pbsubstreams.Clock{
			Id:        item.BlockId,
			Number:    uint64(item.BlockNum),
			Timestamp: item.Timestamp,
		}, item.Payload); err != nil {
			return err
		}
	}
	return newFile.Save(ctx)
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

		// Limit reading to MaxExecoutSegmentSize to prevent excessive memory usage
		limitedReader := io.LimitReader(objectReader, int64(MaxExecoutSegmentSize+1))
		bytes, err := io.ReadAll(limitedReader)
		if err != nil {
			return fmt.Errorf("reading store file %s: %w", filename, err)
		}
		if len(bytes) == MaxExecoutSegmentSize+1 {
			return derr.NewFatalError(fmt.Errorf("%w: file %s on module %s", ErrSegmentSizeExceeded, filename, c.ModuleName))
		}

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

func (f *File) WriteAsYouGo(ctx context.Context) {
	filename := f.Filename()
	f.logger.Info("begin writing execution output file", zap.String("filename", filename))
	r, w := io.Pipe()
	f.writingFile = w
	f.writeError = make(chan error, 1)

	go func() {
		<-ctx.Done()
		w.CloseWithError(ctx.Err()) // this will trigger an error in 'store.WriteObject' in next thread. NOOP if already closed
	}()
	go func() {
		// writes the data from the pipe to the storage
		// any error here closes the pipe (to fail on next write)
		// and also lgets written to the writeError channel for 'Save' operation to pick up
		err := f.store.WriteObject(ctx, filename, r)
		if err != nil && !errors.Is(err, context.Canceled) {
			f.logger.Warn("error writing execution output file", zap.String("filename", filename), zap.Error(err))
		}
		w.CloseWithError(err) // NOOP if already closed

		f.writeError <- err // so the "Save" operation can wait on write completion and determine if something failed
		close(f.writeError)
	}()
}

func (c *File) Save(ctx context.Context) error {
	if c.writingFile == nil {
		return fmt.Errorf("cannot save file %s: writingfile is nil", c.Filename())
	}
	c.writePreviousKV() // last block output must be written
	if err := c.writingFile.Close(); err != nil {
		return fmt.Errorf("closing file %s: %w", c.Filename(), err)
	}
	return <-c.writeError
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
