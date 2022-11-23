package execout

import (
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
	files             map[string]*File // moduleName => file
	exclusiveEndBlock uint64
	outputModules     map[string]bool
	configs           *Configs
}

func NewExecOutputWriter(initialBlockBoundary, exclusiveEndBlock uint64, outputModules map[string]bool, configs *Configs) *ExecOutputWriter {
	w := &ExecOutputWriter{
		files:             make(map[string]*File),
		configs:           configs,
		exclusiveEndBlock: exclusiveEndBlock,
		outputModules:     outputModules,
	}

	for modName := range w.outputModules {
		modInitBlock := configs.ConfigMap[modName].ModuleInitialBlock()
		rng := block.NewBoundedRange(modInitBlock, configs.execOutputSaveInterval, initialBlockBoundary, exclusiveEndBlock)
		newFile := configs.NewFile(modName, targetRange)
		w.files[modName] = newFile
	}
}

func (w *ExecOutputWriter) Write(clock *pbsubstreams.Clock, buffer *ExecOutputBuffer) error {
	// rotate files?
	for modName := range w.outputModules {
		curFile := w.files[modName]
		if curFile.IsOutOfBounds(clock.Number) {
			targetRange := computeNextBounds(curFile.targetRange, w.exclusiveEndBlock, w.configs.execOutputSaveInterval)
			newFile := w.configs.NewFile(modName, targetRange)
		}

		buffer.values[modName]
	}
}

func (w *ExecOutputWriter) Close() error {
	// TODO(abourget): make sure we flush and wait for all the Save()'s to happen
}
