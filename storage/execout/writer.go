package execout

import (
	"context"
	"fmt"
	"sync"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// MultiplexedWriter?

// The Writer is responsible for writing an rotating files
// containing execution outputs.
// Those files will then be read by the LinearExecOutReader.
// `initialBlockBoundary` is expected to be on a boundary, or to be
// modules' initial blocks.
type Writer struct {
	wg *sync.WaitGroup

	fileWalker   *FileWalker
	currentFile  *File
	outputModule string
}

func NewWriter(initialBlockBoundary, exclusiveEndBlock uint64, outputModule string, configs *Configs) *Writer {
	w := &Writer{
		wg:           &sync.WaitGroup{},
		outputModule: outputModule,
	}

	segmenter := block.NewSegmenter(configs.execOutputSaveInterval, initialBlockBoundary, exclusiveEndBlock)
	walker := configs.NewFileWalker(outputModule, segmenter)
	w.fileWalker = walker
	w.currentFile = walker.File()

	return w
}

func (w *Writer) Write(clock *pbsubstreams.Clock, buffer *Buffer) {
	if val, found := buffer.values[w.outputModule]; found {
		w.currentFile.SetItem(clock, val)
	}
}

func (w *Writer) MaybeRotate(ctx context.Context, clockNumber uint64) error {
	if w.currentFile == nil {
		return nil
	}

	if w.currentFile.IsOutOfBounds(clockNumber) { // bounds are per file, because module init are per module
		doSave, err := w.currentFile.Save(ctx)
		if err != nil {
			return fmt.Errorf("flushing exec output writer: %w", err)
		}
		w.wg.Add(1)
		go func() {
			doSave()
			w.wg.Done()
		}()

		for {
			// Support skipped blocks, if we jump 500 blocks here, we're still going to be in the right file.
			w.fileWalker.Next()
			if w.fileWalker.IsDone() {
				break
			}

			w.currentFile = w.fileWalker.File()

			if !w.currentFile.Contains(clockNumber) {
				doSave, err := w.currentFile.Save(ctx)
				if err != nil {
					return fmt.Errorf("saving skipped file: %w", err)
				}
				doSave()
				continue
			}
			break
		}
	}
	return nil
}

func (w *Writer) Close() {
	w.wg.Wait()
}
