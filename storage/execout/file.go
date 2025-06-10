package execout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"github.com/streamingfast/substreams/storage/execout/streamproto"

	"go.uber.org/zap/zapcore"

	"github.com/streamingfast/dstore"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type FileReader interface {
	ReadNext() (*pboutput.Item, error)
	Get(ctx context.Context, blockNumber uint64) (payload []byte, found bool, err error)
	ModuleName() string
	Filename() string
}

type fileReader struct {
	*File
	reader       io.ReadCloser
	lastReadItem *pboutput.Item
	complete     bool
}

func NewFileReader(ctx context.Context, store dstore.Store, logger *zap.Logger, rng *block.Range, moduleName string) (FileReader, error) {
	fr := &fileReader{
		File: &File{
			Range:      rng,
			moduleName: moduleName,
			store:      store,
			logger:     logger,
		},
	}
	err := fr.open(ctx)
	return fr, err
}

type FileWriter interface {
	SetItem(clock *pbsubstreams.Clock, data []byte) error
	Close() error
	ModuleName() string
	Filename() string
	Range() *block.Range
}

type fileWriter struct {
	*File
	writer             *io.PipeWriter
	lastWrittenItem    *pboutput.Item
	orderedFlagWritten bool
	writeError         chan error
	done               chan struct{}
}

func (fw *fileWriter) Range() *block.Range {
	return fw.File.Range
}

type ClockDistributor struct {
	execOuts          map[string]FileReader
	execOutsLastclock map[string]uint64
	seenClocks        map[uint64]*pbsubstreams.Clock
	startBlock        uint64
	stopBlock         uint64
	nextClockNumber   uint64
}

func NewClockDistributor(execOuts map[string]FileReader, startBlock uint64, stopBlock uint64) *ClockDistributor {
	return &ClockDistributor{
		execOuts:          execOuts,
		execOutsLastclock: make(map[string]uint64),
		seenClocks:        make(map[uint64]*pbsubstreams.Clock),
		startBlock:        startBlock,
		stopBlock:         stopBlock,
		nextClockNumber:   startBlock,
	}
}

func (cd *ClockDistributor) Next(ctx context.Context) (*pbsubstreams.Clock, error) {
	for i := cd.nextClockNumber; i < cd.stopBlock; i++ {
		for name, execOut := range cd.execOuts {
			for {
				lastRead, ok := cd.execOutsLastclock[name]
				if ok && lastRead >= i {
					break
				}
				item, err := execOut.ReadNext()
				if err == io.EOF {
					cd.execOutsLastclock[name] = cd.stopBlock
					break
				}
				if err != nil {
					return nil, err
				}
				cd.seenClocks[item.BlockNum] = &pbsubstreams.Clock{
					Number:    item.BlockNum,
					Id:        item.BlockId,
					Timestamp: item.Timestamp,
				}
				cd.execOutsLastclock[name] = item.BlockNum
			}
		}

		if cd.seenClocks[i] != nil {
			cd.nextClockNumber = i + 1
			return cd.seenClocks[i], nil
		}

	}
	return nil, io.EOF
}

func NewFileWriter(ctx context.Context, store dstore.Store, logger *zap.Logger, rng *block.Range, moduleName string) FileWriter {
	fw := &fileWriter{
		File: &File{
			Range:      rng,
			moduleName: moduleName,
			store:      store,
			logger:     logger,
		},
		orderedFlagWritten: false,
		writeError:         make(chan error, 1),
		done:               make(chan struct{}),
	}

	filename := fw.Filename()
	fw.logger.Info("begin writing execution output file", zap.String("filename", filename))
	r, w := io.Pipe()
	fw.writer = w
	fw.writeError = make(chan error, 1)

	go func() {
		select {
		case <-ctx.Done():
			w.CloseWithError(ctx.Err()) // this will trigger an error in 'store.WriteObject' in next thread. NOOP if already closed
		case <-fw.done:
		}
	}()
	go func() {
		// writes the data from the pipe to the storage
		// any error here closes the pipe (to fail on next write)
		// and also lgets written to the writeError channel for 'Save' operation to pick up
		err := fw.store.WriteObject(ctx, filename, r)
		if err != nil && !errors.Is(err, context.Canceled) {
			fw.logger.Warn("error writing execution output file", zap.String("filename", filename), zap.Error(err))
		}
		w.CloseWithError(err) // NOOP if already closed

		fw.writeError <- err // so the "Save" operation can wait on write completion and determine if something failed
		close(fw.writeError)
	}()
	return fw
}

// A File in `execout` stores, for a given module (with a given hash), the outputs of module execution
// for _multiple blocks_, based on their block ID.
type File struct {
	*block.Range
	moduleName string
	store      dstore.Store
	logger     *zap.Logger
}

func (c *File) FullFilename() string {
	return path.Join(c.store.BaseURL().String(), c.Filename())
}
func (c *File) Filename() string {
	return computeDBinFilename(c.Range.StartBlock, c.Range.ExclusiveEndBlock)
}

func (c *File) ModuleName() string {
	return c.moduleName
}

func (fw *fileWriter) SetItem(clock *pbsubstreams.Clock, data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)

	item := &pboutput.Item{
		BlockNum:  clock.Number,
		BlockId:   clock.Id,
		Timestamp: clock.Timestamp,
		Payload:   cp,
	}

	if !fw.orderedFlagWritten {
		if _, err := streamproto.WriteOrderedBool(fw.writer); err != nil {
			return fmt.Errorf("writing ordered bool: %w", err)
		}
		fw.orderedFlagWritten = true
	}

	if _, err := streamproto.WriteItem(fw.writer, item); err != nil {
		return err
	}

	fw.lastWrittenItem = item
	return nil
}

func (fr *fileReader) Get(ctx context.Context, blockNumber uint64) (payload []byte, found bool, err error) {
	next := fr.lastReadItem

	if next == nil && fr.complete { // file has no data
		return nil, false, nil
	}
	for {
		if next != nil {
			switch {
			case next.BlockNum == blockNumber:
				return next.Payload, true, nil
			case next.BlockNum > blockNumber:
				return nil, false, nil
			}
		}
		next, err = fr.ReadNext()
		if err == io.EOF {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}

	}
}

func (fr *fileReader) open(ctx context.Context) error {
	r, err := fr.store.OpenObject(ctx, fr.Filename())
	if err != nil {
		return err
	}
	ordered, readBytes, err := streamproto.ReadOrderedBool(r)
	if err != nil {
		return err
	}
	if !ordered {
		if err := rewriteAsOrdered(ctx, r, readBytes, fr.store, fr.ModuleName(), fr.Filename(), fr.Range, fr.logger); err != nil {
			fmt.Println("rewriting as ordered error", err)
			return err
		}
		if err := r.Close(); err != nil {
			fmt.Println("closing err", err)
			return err
		}

		r, err := fr.store.OpenObject(ctx, fr.Filename())
		if err != nil {
			fmt.Println("opening object error", err)
			return err
		}
		ordered, readBytes, err = streamproto.ReadOrderedBool(r)
		if err != nil {
			return err
		}
		if !ordered {
			return fmt.Errorf("internal error: could not rewrite outputs as ordered")
		}
		fr.reader = r
	} else {
		fr.reader = r
	}

	return nil
}

func (fr *fileReader) ReadNext() (*pboutput.Item, error) {
	if fr.complete {
		return nil, io.EOF
	}
	item, err := streamproto.ReadNextItem(fr.reader)
	if err != nil {
		if err == io.EOF {
			fr.complete = true
			return nil, io.EOF
		}
		return nil, err
	}
	return item, nil
}

func rewriteAsOrdered(ctx context.Context, r io.Reader, readBytes []byte, store dstore.Store, moduleName, filename string, rng *block.Range, logger *zap.Logger) error {
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // closes the WriteAsYouGo
	newFile := NewFileWriter(ctx, store, logger, rng, moduleName)

	for _, item := range items {
		if err := newFile.SetItem(&pbsubstreams.Clock{
			Id:        item.BlockId,
			Number:    uint64(item.BlockNum),
			Timestamp: item.Timestamp,
		}, item.Payload); err != nil {
			return err
		}
	}
	return newFile.Close()
}

func (fw *fileWriter) Close() error {
	if err := fw.writer.Close(); err != nil {
		return fmt.Errorf("closing file %s: %w", fw.Filename(), err)
	}
	close(fw.done)
	return <-fw.writeError // error at the other end of the pipe
}

func (c *File) String() string {
	return c.store.ObjectURL(c.Filename())
}

func (c *File) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if c == nil {
		return nil
	}
	enc.AddString("module", c.ModuleName())
	enc.AddUint64("start_block", c.Range.StartBlock)
	enc.AddUint64("end_block", c.Range.ExclusiveEndBlock)
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
