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

	CurrentFile   *File
	outputModule  string
	isIndexWriter bool
}

func NewWriter(initialBlockBoundary, exclusiveEndBlock uint64, outputModule string, configs *Configs, isIndexWriter bool) *Writer {
	w := &Writer{
		wg:            &sync.WaitGroup{},
		outputModule:  outputModule,
		isIndexWriter: isIndexWriter,
	}

	segmenter := block.NewSegmenter(configs.execOutputSaveInterval, initialBlockBoundary, exclusiveEndBlock)
	walker := configs.NewFileWalker(outputModule, segmenter)
	w.CurrentFile = walker.File()

	return w
}

func (w *Writer) Write(clock *pbsubstreams.Clock, buffer *Buffer) {
	if val, found := buffer.valuesForFileOutput[w.outputModule]; found {
		w.CurrentFile.SetItem(clock, val)
	}
}

func (w *Writer) Close(ctx context.Context) error {
	// Skip outputs file saving for blockIndex module
	if w.isIndexWriter {
		return nil
	}

	if err := w.CurrentFile.Save(ctx); err != nil {
		return fmt.Errorf("flushing exec output writer: %w", err)
	}
	return nil
}
