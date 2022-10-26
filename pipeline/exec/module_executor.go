package exec

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/metrics"
	"github.com/streamingfast/substreams/reqctx"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
)

func RunModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutput, requestStats metrics.Stats) (*pbsubstreams.ModuleOutput, error) {
	logger := reqctx.Logger(ctx)

	var err error

	ctx, span := reqctx.WithSpan(ctx, "module_execution")
	defer span.EndWithErr(&err)

	logger = logger.With(zap.String("module_name", executor.Name()))
	span.SetAttributes(attribute.String("module.name", executor.Name()))

	logger.Debug("running module")

	cached, output, err := cacheOutputExists(execOutput, executor)
	if err != nil {
		return nil, fmt.Errorf("check cache output exists: %w", err)
	}
	span.SetAttributes(attribute.Bool("module.cached", cached))

	if cached {
		requestStats.RecordOutputCacheHit()
		if err = executor.applyCachedOutput(output); err != nil {
			return nil, fmt.Errorf("apply cached output: %w", err)
		}
		return nil, nil
	}
	requestStats.RecordOutputCacheMiss()

	outputBytes, moduleOutput, err := executeModule(ctx, executor, execOutput)
	if err != nil {
		return moduleOutput, fmt.Errorf("execute: %w", err)
	}

	hasValidOutput := moduleOutput != nil
	if hasValidOutput {
		if err = setOutputCache(executor.Name(), execOutput, outputBytes); err != nil {
			return moduleOutput, fmt.Errorf("set output cache: %w", err)
		}
	}

	return moduleOutput, nil
}

func cacheOutputExists(execOutput execout.ExecutionOutput, executor ModuleExecutor) (bool, []byte, error) {
	output, cached, err := execOutput.Get(executor.Name())
	if err != nil && err != execout.NotFound {
		return false, nil, fmt.Errorf("get cached output: %w", err)
	}
	return cached, output, nil
}

func executeModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutput) ([]byte, *pbsubstreams.ModuleOutput, error) {
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
		Name:          executor.Name(),
		Logs:          logs,
		LogsTruncated: truncated,
		Data:          data,
	}
	return output
}

func setOutputCache(executorName string, execOutput execout.ExecutionOutput, value []byte) error {
	err := execOutput.Set(executorName, value)
	if err != nil {
		return fmt.Errorf("set cached output: %w", err)
	}
	return nil
}
