package exec

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/streamingfast/substreams/metrics"
	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
)

type MockExecOutput struct {
	clockFunc func() *pbsubstreams.Clock

	cacheMap map[string][]byte
}

func (t *MockExecOutput) Clock() *pbsubstreams.Clock {
	return t.clockFunc()
}

func (t *MockExecOutput) Len() int {
	return 0
}

func (t *MockExecOutput) Get(name string) ([]byte, bool, error) {
	v, ok := t.cacheMap[name]
	if !ok {
		return nil, false, execout.ErrNotFound
	}
	return v, true, nil
}

func (t *MockExecOutput) Set(name string, value []byte) (err error) {
	t.cacheMap[name] = value
	return nil
}

type MockModuleExecutor struct {
	name string

	RunFunc      func(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error)
	ApplyFunc    func(value []byte) error
	LogsFunc     func() (logs []string, truncated bool)
	StackFunc    func() []string
	ToOutputFunc func(data []byte) (*pbssinternal.ModuleOutput, error)
	cacheable    bool
}

var _ ModuleExecutor = (*MockModuleExecutor)(nil)

func (t *MockModuleExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter, cachable bool) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
	if t.RunFunc != nil {
		return t.RunFunc(ctx, reader)
	}
	return nil, nil, nil, fmt.Errorf("not implemented")
}
func (t *MockModuleExecutor) BlockIndex() *index.BlockIndex   { return nil }
func (t *MockModuleExecutor) RunsOnBlock(_ uint64) bool       { return true }
func (t *MockModuleExecutor) Name() string                    { return t.name }
func (t *MockModuleExecutor) String() string                  { return fmt.Sprintf("TestModuleExecutor(%s)", t.name) }
func (t *MockModuleExecutor) Close(ctx context.Context) error { return nil }
func (t *MockModuleExecutor) HasValidOutput() bool            { return t.cacheable }
func (t *MockModuleExecutor) HasOutputForFiles() bool         { return false }

func (t *MockModuleExecutor) applyCachedOutput(value []byte) error {
	if t.ApplyFunc != nil {
		return t.ApplyFunc(value)
	}
	return fmt.Errorf("not implemented")
}

func (t *MockModuleExecutor) toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error) {
	if t.ToOutputFunc != nil {
		return t.ToOutputFunc(data)
	}
	return nil, fmt.Errorf("not implemented")
}

func (t *MockModuleExecutor) lastExecutionLogs() (logs []string, truncated bool) {
	if t.LogsFunc != nil {
		return t.LogsFunc()
	}
	return nil, false
}

func TestModuleExecutorRunner_Run_HappyPath(t *testing.T) {
	ctx := context.Background()

	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))
	executor := &MockModuleExecutor{
		name: "test",
		RunFunc: func(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
			return []byte("test"), nil, &pbssinternal.ModuleOutput{
				Data: &pbssinternal.ModuleOutput_MapOutput{
					MapOutput: nil,
				},
			}, nil
		},
		LogsFunc: func() (logs []string, truncated bool) {
			return []string{"test"}, false
		},
	}
	output := &MockExecOutput{
		cacheMap: make(map[string][]byte),
	}

	moduleOutput, _, _, _, _, err := RunModule(ctx, executor, output, true)
	if err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, err)
	assert.NotEmpty(t, moduleOutput)
}

func TestModuleExecutorRunner_Run_CachedOutput(t *testing.T) {
	ctx := context.Background()

	applied := false

	executor := &MockModuleExecutor{
		name: "test",
		RunFunc: func(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
			return []byte("test"), nil, &pbssinternal.ModuleOutput{
				Data: &pbssinternal.ModuleOutput_MapOutput{
					MapOutput: nil,
				},
			}, nil
		},
		ToOutputFunc: func(data []byte) (*pbssinternal.ModuleOutput, error) {
			return &pbssinternal.ModuleOutput{
				Data: &pbssinternal.ModuleOutput_MapOutput{
					MapOutput: nil,
				},
			}, nil
		},
		ApplyFunc: func(value []byte) error {
			applied = true
			return nil
		},
		LogsFunc: func() (logs []string, truncated bool) {
			return []string{"test"}, false
		},
	}
	output := &MockExecOutput{
		cacheMap: map[string][]byte{
			"test": []byte("cached"),
		},
	}

	moduleOutput, _, _, _, _, err := RunModule(ctx, executor, output, true)
	if err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, err)
	assert.True(t, applied)
	assert.NotEmpty(t, moduleOutput)
	assert.True(t, moduleOutput.Cached)
}

func TestModuleExecutorRunner_Run_NoInput_HasProperTypeUrl(t *testing.T) {
	// This test verifies the fix for the issue where ErrNoInput caused an empty anypb.Any
	// (no TypeUrl, no Value) to be sent in BlockScopedData responses.
	// When a module has no input for a block, the returned moduleOutput should still
	// have the correct TypeUrl set, matching the behaviour of the skipFromIndex path.
	ctx := context.Background()
	ctx = reqctx.WithReqStats(ctx, metrics.NewReqStats(&metrics.Config{}, nil, nil, zap.NewNop()))

	const expectedTypeUrl = "type.googleapis.com/test.MyOutput"

	executor := &MockModuleExecutor{
		name:      "test_module",
		cacheable: true,
		RunFunc: func(ctx context.Context, reader execout.ExecutionOutputGetter) (out []byte, outForFiles []byte, moduleOutputData *pbssinternal.ModuleOutput, err error) {
			return nil, nil, nil, ErrNoInput
		},
		ToOutputFunc: func(data []byte) (*pbssinternal.ModuleOutput, error) {
			return &pbssinternal.ModuleOutput{
				Data: &pbssinternal.ModuleOutput_MapOutput{
					MapOutput: &anypb.Any{TypeUrl: expectedTypeUrl, Value: data},
				},
			}, nil
		},
	}
	output := &MockExecOutput{
		cacheMap: make(map[string][]byte),
	}

	moduleOutput, outputBytes, _, skippedExecution, skippableOutput, err := RunModule(ctx, executor, output, true)

	assert.NoError(t, err)
	assert.True(t, skippedExecution, "execution should be marked as skipped")
	assert.True(t, skippableOutput, "output should be marked as skippable")
	assert.Nil(t, outputBytes, "no bytes should be returned for skipped execution")

	// The key assertion: moduleOutput must NOT be nil and must have the TypeUrl set,
	// preventing an empty anypb.Any from being sent to clients.
	require.NotNil(t, moduleOutput, "moduleOutput must not be nil even when ErrNoInput")
	require.NotNil(t, moduleOutput.GetMapOutput(), "MapOutput must not be nil")
	assert.Equal(t, expectedTypeUrl, moduleOutput.GetMapOutput().TypeUrl, "TypeUrl must be set even for empty outputs")
	assert.Nil(t, moduleOutput.GetMapOutput().Value, "Value should be nil/empty for no-input outputs")
	assert.Equal(t, "test_module", moduleOutput.ModuleName, "ModuleName must be populated")
}
