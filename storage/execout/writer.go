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

	files        map[string]*File // moduleName => file
	outputModule string
	configs      *Configs
}

func NewWriter(initialBlockBoundary, exclusiveEndBlock uint64, outputModule string, configs *Configs, isSubRequest bool) *Writer {
	w := &Writer{
		wg:           &sync.WaitGroup{},
		files:        make(map[string]*File),
		configs:      configs,
		outputModule: outputModule,
	}

	modInitBlock := configs.ConfigMap[outputModule].ModuleInitialBlock()
	var upperBound uint64
	if isSubRequest {
		upperBound = exclusiveEndBlock
	} else {
		// Push to the next boundary, so nothing would get flushed at the requested stop block boundary.
		upperBound = exclusiveEndBlock - exclusiveEndBlock%configs.execOutputSaveInterval + configs.execOutputSaveInterval
	}
	targetRange := block.NewBoundedRange(modInitBlock, configs.execOutputSaveInterval, initialBlockBoundary, upperBound)
	newFile := configs.NewFile(outputModule, targetRange)
	w.files[outputModule] = newFile

	return w
}

func (w *Writer) Write(clock *pbsubstreams.Clock, buffer *Buffer) {
	if val, found := buffer.values[w.outputModule]; found {
		// TODO(abourget): triple check that we don't want to write
		// if not found?
		curFile, found := w.files[w.outputModule]
		if found {
			curFile.SetItem(clock, val)
		}
	}
}

func (w *Writer) MaybeRotate(ctx context.Context, clockNumber uint64) error {
	curFile := w.files[w.outputModule]
	if curFile == nil {
		return nil
	}
	if curFile.IsOutOfBounds(clockNumber) { // bounds are per file, because module init are per module
		doSave, err := curFile.Save(ctx)
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
			curFile = curFile.NextFile()
			if curFile == nil {
				break
			}
			if !curFile.Contains(clockNumber) {
				doSave, err := curFile.Save(ctx)
				if err != nil {
					return fmt.Errorf("saving skipped file: %w", err)
				}
				doSave()
				continue
			}
			break
		}

		if curFile == nil {
			delete(w.files, w.outputModule)
		} else {
			w.files[w.outputModule] = curFile
		}
	}
	return nil
}

func (w *Writer) Close() {
	w.wg.Wait()
}
