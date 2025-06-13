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

	CurrentFile      FileWriter
	outputModule     string
	isWriterForIndex bool
}

func NewWriter(ctx context.Context, initialBlockBoundary, exclusiveEndBlock uint64, outputModule string, configs *Configs) *Writer {
	w := &Writer{
		wg:           &sync.WaitGroup{},
		outputModule: outputModule,
	}

	segmenter := block.NewSegmenter(configs.execOutputSaveInterval, initialBlockBoundary, exclusiveEndBlock)
	walker := configs.NewFileWalker(outputModule, segmenter)
	w.CurrentFile = walker.FileWriter(ctx)

	return w
}

func (w *Writer) Write(clock *pbsubstreams.Clock, buffer *Buffer) error {
	if val, found := buffer.valuesForFileOutput[w.outputModule]; found {
		if err := w.CurrentFile.SetItem(clock, val); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) Close() error {
	// Skip outputs file saving for blockIndex module
	if w.isWriterForIndex {
		return nil
	}

	if err := w.CurrentFile.Close(); err != nil {
		return fmt.Errorf("flushing exec output writer: %w", err)
	}
	return nil
}
