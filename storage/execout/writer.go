package execout

import (
	"context"
	"fmt"
	"sync"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// The Writer writes a single file with executionOutputs that will be read by the LinearExecOutReader.
// `initialBlockBoundary` is expected to be on a boundary, or to be the module's initial block.
type Writer struct {
	wg *sync.WaitGroup

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
	w.currentFile = walker.File()

	return w
}

func (w *Writer) Write(clock *pbsubstreams.Clock, buffer *Buffer) {
	if val, found := buffer.values[w.outputModule]; found {
		w.currentFile.SetItem(clock, val)
	}
}

func (w *Writer) Close(ctx context.Context) error {
	if err := w.currentFile.Save(ctx); err != nil {
		return fmt.Errorf("flushing exec output writer: %w", err)
	}
	return nil
}
