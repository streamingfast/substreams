package exec

import (
	"context"
	"fmt"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/execout"
)

type ModuleExecutorRunner struct {
	executor ModuleExecutor
	output   execout.ExecutionOutput
}

func RunModule(ctx context.Context, executor ModuleExecutor, execOutput execout.ExecutionOutput) (*pbsubstreams.ModuleOutput, error) {
	cached, output, err := cacheOutputExists(execOutput, executor)
	if err != nil {
		return nil, fmt.Errorf("check cache output exists: %w", err)
	}

	if cached {
		if err = executor.applyCachedOutput(output); err != nil {
			return nil, fmt.Errorf("apply cached output: %w", err)
		}
		return nil, nil
	}

	outputBytes, moduleOutput, err := executeModule(ctx, executor, execOutput)
	if err != nil {
		return moduleOutput, fmt.Errorf("execute: %w", err)
	}

	if err = setOutputCache(executor, execOutput, outputBytes); err != nil {
		return moduleOutput, fmt.Errorf("set output cache: %w", err)
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

func setOutputCache(executor ModuleExecutor, execOutput execout.ExecutionOutput, value []byte) error {
	err := execOutput.Set(executor.Name(), value)
	if err != nil {
		return fmt.Errorf("set cached output: %w", err)
	}
	return nil
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
