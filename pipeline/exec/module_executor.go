package exec

import (
	"context"
	"errors"
	"fmt"

	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"google.golang.org/protobuf/proto"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func skipFromIndex(index *index.BlockIndex, execOutput execout.ExecutionOutputGetter) bool {
	if index == nil {
		return false
	}

	if index.Precomputed() {
		return index.Skip(uint64(execOutput.Clock().Number))
	}

	indexedKeys, _, err := execOutput.Get(index.IndexModule)
	if err != nil {
		if err == execout.ErrNotFound {
			return true // an empty index means we should skip
		}
		panic(fmt.Errorf("getting index module output for keys: %w", err))
	}

	return index.SkipFromKeys(indexedKeys)

}

// RunModule returns:
//   - moduleOutput:     the structured data
//   - outputBytes:      the marshalled version of moduleOutput
//   - outputBytesFiles: the marshalled bytes to be sent to a file,
//   - skippedExecution: an indication that execution was not performed (skipped from index or skipped because it has no input -- cached data does not count as skipped, but marks the moduleOuput as 'Cached')
//   - skippableOutput:  an indication that the output can be skipped
//   - error:            an error
func RunModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutputGetter, cachable bool) (*pbssinternal.ModuleOutput, []byte, []byte, bool, bool, error) {
	logger := reqctx.Logger(ctx)
	modName := executor.Name()

	var err error
	var skippableOutput bool

	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "module_execution")
	defer span.EndWithErr(&err)

	logger = logger.With(zap.String("module_name", modName))
	span.SetAttributes(attribute.String("substreams.module.name", modName))

	logger.Debug("running module")

	if skipFromIndex(executor.BlockIndex(), execOutput) {
		emptyOutput, _ := executor.toModuleOutput(nil)

		if emptyOutput == nil {
			// if the moduleOutput is nil here, we may be on a module type that never produces outputs, like a store on partialKV
			return nil, nil, nil, true, true, nil
		}

		// always set the module name here to play well with sinks
		emptyOutput.ModuleName = executor.Name()
		return emptyOutput, nil, nil, true, true, nil
	}

	cached, outputBytes, err := getCachedOutput(execOutput, executor)
	if err != nil {
		return nil, nil, nil, false, skippableOutput, fmt.Errorf("check cache output exists: %w", err)
	}
	span.SetAttributes(attribute.Bool("substreams.module.cached", cached))

	if cached {
		if err = executor.applyCachedOutput(outputBytes); err != nil {
			return nil, nil, nil, false, skippableOutput, fmt.Errorf("apply cached output: %w", err)
		}

		moduleOutput, err := executor.toModuleOutput(outputBytes)
		if err != nil {
			return moduleOutput, outputBytes, nil, false, skippableOutput, fmt.Errorf("converting output to module output: %w", err)
		}

		if moduleOutput == nil {
			return nil, nil, nil, false, skippableOutput, nil // output from PartialKV is always nil, we cannot use it
		}

		// For store modules, the output in cache is in "operations", but we get the proper store deltas in "toModuleOutput", so we need to transform back those deltas into outputBytes
		if storeDeltas := moduleOutput.GetStoreDeltas(); storeDeltas != nil {
			outputBytes, err = proto.Marshal(moduleOutput.GetStoreDeltas())
			if err != nil {
				return nil, nil, nil, false, skippableOutput, err
			}
		}

		fillModuleOutputMetadata(executor, moduleOutput)
		moduleOutput.Cached = true
		return moduleOutput, outputBytes, nil, false, skippableOutput, nil
	}

	stats := reqctx.ReqStats(ctx)
	uid := stats.RecordModuleWasmBlockBegin(modName)
	defer stats.RecordModuleWasmBlockEnd(modName, uid)

	outputBytes, outputForFiles, moduleOutput, err := executor.run(ctx, execOutput, cachable)
	switch {
	case errors.Is(err, ErrSkippableOutput):
		skippableOutput = true
	case errors.Is(err, ErrNoInput):
		// Follow the same pattern as skipFromIndex: call toModuleOutput(nil) to get an output
		// with the TypeUrl set, even though there is no value. This prevents an empty anypb.Any{}
		// (no TypeUrl, no Value) from being sent to clients in the BlockScopedData response.
		// The error is intentionally discarded here since toModuleOutput(nil) does not error
		// for map/index modules, matching the skipFromIndex path above.
		emptyOutput, _ := executor.toModuleOutput(nil)
		if emptyOutput != nil {
			emptyOutput.ModuleName = executor.Name()
		}
		return emptyOutput, nil, nil, true, true, nil
	case err != nil:
		return nil, nil, nil, false, skippableOutput, fmt.Errorf("execute: %w", err)
	}

	fillModuleOutputMetadata(executor, moduleOutput)

	return moduleOutput, outputBytes, outputForFiles, false, skippableOutput, nil
}

func getCachedOutput(execOutput execout.ExecutionOutputGetter, executor ModuleExecutor) (bool, []byte, error) {
	output, cached, err := execOutput.Get(executor.Name())
	if err != nil && err != execout.ErrNotFound {
		return false, nil, fmt.Errorf("get cached output: %w", err)
	}
	return cached, output, nil
}

func fillModuleOutputMetadata(executor ModuleExecutor, in *pbssinternal.ModuleOutput) {
	logs, truncated := executor.lastExecutionLogs()

	in.ModuleName = executor.Name()
	in.Logs = logs
	in.DebugLogsTruncated = truncated
}
