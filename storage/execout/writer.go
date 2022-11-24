package execout

import (
	"context"
	"fmt"
	"sync"

	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// MultiWriter ?

// The ExecOutputWriter is responsible for writing an rotating files
// containing execution outputs.
// Those files will then be read by the LinearExecOutReader.
// `initialBlockBoundary` is expected to be on a boundary, or to be
// modules' initial blocks.
type ExecOutputWriter struct {
	wg *sync.WaitGroup

	files         map[string]*File // moduleName => file
	outputModules map[string]bool
	configs       *Configs
}

func NewExecOutputWriter(initialBlockBoundary, exclusiveEndBlock uint64, outputModules map[string]bool, configs *Configs, isSubRequest bool) *ExecOutputWriter {
	w := &ExecOutputWriter{
		wg:            &sync.WaitGroup{},
		files:         make(map[string]*File),
		configs:       configs,
		outputModules: outputModules,
	}

	for modName := range w.outputModules {
		modInitBlock := configs.ConfigMap[modName].ModuleInitialBlock()
		var upperBound uint64
		if isSubRequest {
			upperBound = exclusiveEndBlock
		} else {
			// Push to the next boundary, so nothing would get flushed at the requested stop block boundary.
			upperBound = exclusiveEndBlock - exclusiveEndBlock%configs.execOutputSaveInterval + configs.execOutputSaveInterval
		}
		targetRange := block.NewBoundedRange(modInitBlock, configs.execOutputSaveInterval, initialBlockBoundary, upperBound)
		newFile := configs.NewFile(modName, targetRange)
		w.files[modName] = newFile
	}

	return w
}

func (w *ExecOutputWriter) Write(clock *pbsubstreams.Clock, buffer *ExecOutputBuffer) {
	for modName := range w.outputModules {
		if val, found := buffer.values[modName]; found {
			// TODO(abourget): triple check that we don't want to write
			// if not found?
			curFile, found := w.files[modName]
			if found {
				curFile.SetItem(clock, val)
			}
		}
	}
}

func (w *ExecOutputWriter) MaybeRotate(ctx context.Context, clockNumber uint64) error {
	for modName := range w.outputModules {
		curFile := w.files[modName]
		if curFile == nil {
			continue
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
				delete(w.files, modName)
			} else {
				w.files[modName] = curFile
			}
		}
	}
	return nil
}

func (w *ExecOutputWriter) Close() error {
	// TODO(abourget): make sure we flush and wait for all the Save()'s to happen
	w.wg.Wait()
	return nil
}
