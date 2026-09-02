package execout

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"path"
	"slices"
	"strconv"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"github.com/streamingfast/substreams/storage/execout/streamproto"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type FileReader interface {
	ReadNext() (*pboutput.Item, error)
	Iter() iter.Seq2[*pboutput.Item, error]
	Get(ctx context.Context, blockNumber uint64) (payload []byte, found bool, err error)
	ModuleName() string
	Filename() string
	Close() error
}

type fileReader struct {
	*File
	reader       io.ReadCloser
	lastReadItem *pboutput.Item
	complete     bool
}

func (fr *fileReader) Close() error {
	return fr.reader.Close()
}

func OpenFileReader(ctx context.Context, store dstore.Store, logger *zap.Logger, rng *block.Range, moduleName string) (FileReader, error) {
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
	payloadBytes       uint64 // uncompressed size of the payloads written so far
	itemCount          uint64 // number of items written so far
}

func (fw *fileWriter) Range() *block.Range {
	return fw.File.Range
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
	fw.payloadBytes += uint64(len(cp))
	fw.itemCount++
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
			return err
		}

		r, err := fr.store.OpenObject(ctx, fr.Filename())
		if err != nil {
			return err
		}
		ordered, readBytes, err = streamproto.ReadOrderedBool(r)
		if err != nil {
			return err
		}

		// We check again here because we have to read that boolean anyway, might as well check it. If a race condition or a store bad config with 'Overwriting' gives us the old file again, we have to stop here.
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
	fr.lastReadItem = item
	return item, nil
}

// Iter returns an iterator that yields all items from the reader.
// Usage: for item, err := range fileReader.Iter() { ... }
func (fr *fileReader) Iter() iter.Seq2[*pboutput.Item, error] {
	return func(yield func(*pboutput.Item, error) bool) {
		for {
			item, err := fr.ReadNext()
			if err == io.EOF { // DONE
				return
			}
			if !yield(item, err) {
				return
			}
		}
	}
}

// rewriteAsOrdered reads the file (prepending the already-read-bytes), closes it, then overwrites it with the ordered data.
func rewriteAsOrdered(ctx context.Context, r io.ReadCloser, readBytes []byte, store dstore.Store, moduleName, filename string, rng *block.Range, logger *zap.Logger) (err error) {
	bytes, err := io.ReadAll(r)
	if err != nil {
		r.Close() // always close that reader to prevent leaking unzstd threads
		return fmt.Errorf("reading store file %s: %w", filename, err)
	}
	if err := r.Close(); err != nil {
		return err
	}

	bytes = append(readBytes, bytes...)

	o := &pboutput.Array{}
	if err := o.UnmarshalVTUnsafe(bytes); err != nil {
		return fmt.Errorf("unmarshalling data: %w", err)
	}
	items := o.Items

	slices.SortFunc(items, func(a, b *pboutput.Item) int {
		return cmp.Compare(a.BlockNum, b.BlockNum)
	})

	store.SetOverwrite(true)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // closes the WriteAsYouGo
	newFile := NewFileWriter(ctx, store, logger, rng, moduleName)
	defer func() { err = errors.Join(err, newFile.Close()) }()

	for _, item := range items {
		if err := newFile.SetItem(&pbsubstreams.Clock{
			Id:        item.BlockId,
			Number:    uint64(item.BlockNum),
			Timestamp: item.Timestamp,
		}, item.Payload); err != nil {
			return err
		}
	}

	return nil
}

func (fw *fileWriter) Close() error {
	if err := fw.writer.Close(); err != nil {
		return fmt.Errorf("closing file %s: %w", fw.Filename(), err)
	}
	close(fw.done)
	if err := <-fw.writeError; err != nil { // error at the other end of the pipe
		return err
	}
	fw.setDataSizeMetadata()
	return nil
}

// setDataSizeMetadata records the uncompressed payload size of the file, and how many items
// it holds, as object metadata, so that a reader interested only in what a segment represents
// (the cost estimator) can get both without downloading and decompressing the file. The item
// count is what a consumer receives as messages, which on a module gated by a block index is
// far below the number of blocks the segment covers.
//
// Best-effort and detached: the segment is already written and valid without it, not every
// dstore backend supports metadata at all, and on the backends where setting it means
// rewriting the object it is skipped entirely (see metadataRewriteSchemes). A reader that
// finds no metadata falls back to reading the file.
func (fw *fileWriter) setDataSizeMetadata() {
	objStore, filename, size, items, logger := fw.store, fw.Filename(), fw.payloadBytes, fw.itemCount, fw.logger
	if baseURL := objStore.BaseURL(); baseURL != nil && metadataRewriteSchemes[baseURL.Scheme] {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), setMetadataTimeout)
		defer cancel()
		metadata := map[string]string{
			MetadataDataSize:  strconv.FormatUint(size, 10),
			MetadataItemCount: strconv.FormatUint(items, 10),
		}
		if err := objStore.SetMetadata(ctx, filename, metadata); err != nil {
			logger.Debug("cannot set datasize metadata on execution output file", zap.String("filename", filename), zap.Error(err))
		}
	}()
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
