package execout

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// The Writer writes a single file with executionOutputs that will be read by the LinearExecOutReader.
// `initialBlockBoundary` is expected to be on a boundary, or to be the module's initial block.
type Writer struct {
	wg *sync.WaitGroup

	CurrentFile      *File
	moduleHash       string
	isWriterForIndex bool
}

func NewWriter(initialBlockBoundary, exclusiveEndBlock uint64, moduleHash string, configs *Configs, isWriterForIndex bool) *Writer {
	if strings.HasPrefix(moduleHash, "orca") {
		debug.PrintStack()
	}
	w := &Writer{
		wg:               &sync.WaitGroup{},
		moduleHash:       moduleHash,
		isWriterForIndex: isWriterForIndex,
	}

	segmenter := block.NewSegmenter(configs.execOutputSaveInterval, initialBlockBoundary, exclusiveEndBlock)
	walker := configs.NewFileWalker(moduleHash, segmenter)
	w.CurrentFile = walker.File()

	return w
}

func (w *Writer) Write(clock *pbsubstreams.Clock, buffer *Buffer) {
	if val, found := buffer.valuesForFileOutput[w.moduleHash]; found {
		w.CurrentFile.SetItem(clock, val)
	}
}

func (w *Writer) Close(ctx context.Context) error {
	// Skip outputs file saving for blockIndex module
	if w.isWriterForIndex {
		return nil
	}

	if err := w.CurrentFile.Save(ctx); err != nil {
		return fmt.Errorf("flushing exec output writer: %w", err)
	}
	return nil
}
