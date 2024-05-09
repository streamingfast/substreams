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
		panic(fmt.Errorf("getting index module output for keys: %w", err))
	}

	return index.SkipFromKeys(indexedKeys)

}

func RunModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutputGetter) (*pbssinternal.ModuleOutput, []byte, []byte, bool, error) {
	logger := reqctx.Logger(ctx)
	modName := executor.Name()

	var err error

	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "module_execution")
	defer span.EndWithErr(&err)

	logger = logger.With(zap.String("module_name", modName))
	span.SetAttributes(attribute.String("substreams.module.name", modName))

	logger.Debug("running module")

	if skipFromIndex(executor.BlockIndex(), execOutput) {
		emptyOutput, _ := executor.toModuleOutput(nil)
		return emptyOutput, nil, nil, true, nil
	}

	cached, outputBytes, err := getCachedOutput(execOutput, executor)
	if err != nil {
		return nil, nil, nil, false, fmt.Errorf("check cache output exists: %w", err)
	}
	span.SetAttributes(attribute.Bool("substreams.module.cached", cached))

	if cached {
		if err = executor.applyCachedOutput(outputBytes); err != nil {
			return nil, nil, nil, false, fmt.Errorf("apply cached output: %w", err)
		}

		moduleOutput, err := executor.toModuleOutput(outputBytes)
		if err != nil {
			return moduleOutput, outputBytes, nil, false, fmt.Errorf("converting output to module output: %w", err)
		}

		if moduleOutput == nil {
			return nil, nil, nil, false, nil // output from PartialKV is always nil, we cannot use it
		}

		// For store modules, the output in cache is in "operations", but we get the proper store deltas in "toModuleOutput", so we need to transform back those deltas into outputBytes
		if storeDeltas := moduleOutput.GetStoreDeltas(); storeDeltas != nil {
			outputBytes, err = proto.Marshal(moduleOutput.GetStoreDeltas())
			if err != nil {
				return nil, nil, nil, false, err
			}
		}

		fillModuleOutputMetadata(executor, moduleOutput)
		moduleOutput.Cached = true
		return moduleOutput, outputBytes, nil, false, nil
	}

	uid := reqctx.ReqStats(ctx).RecordModuleWasmBlockBegin(modName)
	outputBytes, outputForFiles, moduleOutput, err := executor.run(ctx, execOutput)
	if err != nil {
		if errors.Is(err, ErrNoInput) {
			return nil, nil, nil, true, nil
		}
		return nil, nil, nil, false, fmt.Errorf("execute: %w", err)
	}
	reqctx.ReqStats(ctx).RecordModuleWasmBlockEnd(modName, uid)

	fillModuleOutputMetadata(executor, moduleOutput)

	return moduleOutput, outputBytes, outputForFiles, false, nil
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
