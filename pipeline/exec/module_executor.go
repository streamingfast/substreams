package exec

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/storage/execout"
	"google.golang.org/protobuf/proto"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/streamingfast/substreams/reqctx"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
)

func RunModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutputGetter) (*pbssinternal.ModuleOutput, []byte, error) {
	logger := reqctx.Logger(ctx)
	modName := executor.Name()

	var err error

	ctx, span := reqctx.WithModuleExecutionSpan(ctx, "module_execution")
	defer span.EndWithErr(&err)

	logger = logger.With(zap.String("module_name", modName))
	span.SetAttributes(attribute.String("substreams.module.name", modName))

	logger.Debug("running module")

	cached, outputBytes, err := getCachedOutput(execOutput, executor)
	if err != nil {
		return nil, nil, fmt.Errorf("check cache output exists: %w", err)
	}
	span.SetAttributes(attribute.Bool("substreams.module.cached", cached))

	if cached {
		if err = executor.applyCachedOutput(outputBytes); err != nil {
			return nil, nil, fmt.Errorf("apply cached output: %w", err)
		}

		moduleOutput, err := executor.toModuleOutput(outputBytes)
		if err != nil {
			return moduleOutput, outputBytes, fmt.Errorf("converting output to module output: %w", err)
		}

		if moduleOutput == nil {
			return nil, nil, nil // output from PartialKV is always nil, we cannot use it
		}

		// For store modules, the output in cache is in "operations", but we get the proper store deltas in "toModuleOutput", so we need to transform back those deltas into outputBytes
		if storeDeltas := moduleOutput.GetStoreDeltas(); storeDeltas != nil {
			outputBytes, err = proto.Marshal(moduleOutput.GetStoreDeltas())
			if err != nil {
				return nil, nil, err
			}
		}

		fillModuleOutputMetadata(executor, moduleOutput)
		moduleOutput.Cached = true
		return moduleOutput, outputBytes, nil
	}

	uid := reqctx.ReqStats(ctx).RecordModuleWasmBlockBegin(modName)
	outputBytes, moduleOutput, err := executor.run(ctx, execOutput)
	if err != nil {
		return nil, nil, fmt.Errorf("execute: %w", err)
	}
	reqctx.ReqStats(ctx).RecordModuleWasmBlockEnd(modName, uid)

	fillModuleOutputMetadata(executor, moduleOutput)

	return moduleOutput, outputBytes, nil
}

func getCachedOutput(execOutput execout.ExecutionOutputGetter, executor ModuleExecutor) (bool, []byte, error) {
	output, cached, err := execOutput.Get(executor.Name())
	if err != nil && err != execout.NotFound {
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
