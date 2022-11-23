package exec

import (
	"context"
	"fmt"

	"github.com/streamingfast/substreams/storage/execout"

	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func RunModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutputGetter) (*pbsubstreams.ModuleOutput, []byte, error) {
	logger := reqctx.Logger(ctx)
	modName := executor.Name()

	reqStats := reqctx.ReqStats(ctx)

	var err error

	ctx, span := reqctx.WithSpan(ctx, "module_execution")
	defer span.EndWithErr(&err)

	logger = logger.With(zap.String("module_name", modName))
	span.SetAttributes(attribute.String("module.name", modName))

	logger.Debug("running module")

	cached, outputBytes, err := getCachedOutput(execOutput, executor)
	if err != nil {
		return nil, nil, fmt.Errorf("check cache output exists: %w", err)
	}
	span.SetAttributes(attribute.Bool("module.cached", cached))

	if cached {
		reqStats.RecordOutputCacheHit()
		if err = executor.applyCachedOutput(outputBytes); err != nil {
			return nil, nil, fmt.Errorf("apply cached output: %w", err)
		}

		moduleOutput, err := executor.toModuleOutput(outputBytes)
		if err != nil {
			return moduleOutput, outputBytes, fmt.Errorf("converting output to module output: %w", err)
		}

		moduleOutput.Cached = true
		return moduleOutput, outputBytes, nil
	}
	reqStats.RecordOutputCacheMiss()

	outputBytes, moduleOutput, err := executeModule(ctx, executor, execOutput)
	if err != nil {
		return nil, nil, fmt.Errorf("execute: %w", err)
	}

	return moduleOutput, outputBytes, nil
}

func getCachedOutput(execOutput execout.ExecutionOutputGetter, executor ModuleExecutor) (bool, []byte, error) {
	output, cached, err := execOutput.Get(executor.Name())
	if err != nil && err != execout.NotFound {
		return false, nil, fmt.Errorf("get cached output: %w", err)
	}
	return cached, output, nil
}

func executeModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutputGetter) ([]byte, *pbsubstreams.ModuleOutput, error) {
	out, moduleOutputData, err := executor.run(ctx, execOutput)
	moduleOutput := toModuleOutput(executor, moduleOutputData)

	if err != nil {
		return out, moduleOutput, fmt.Errorf("execute: %w", err)
	}
	return out, moduleOutput, nil
}

func toModuleOutput(executor ModuleExecutor, data pbsubstreams.ModuleOutputData) *pbsubstreams.ModuleOutput {
	logs, truncated := executor.moduleLogs()
	if len(logs) == 0 && data == nil {
		return nil
	}

	output := &pbsubstreams.ModuleOutput{
		Name:               executor.Name(),
		DebugLogs:          logs,
		DebugLogsTruncated: truncated,
		Data:               data,
	}
	return output
}
