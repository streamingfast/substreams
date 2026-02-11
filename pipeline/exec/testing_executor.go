package exec

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/index"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestingExecutor is a test implementation of ModuleExecutor that tracks calls
// This is exported so it can be used in pipeline tests
type TestingExecutor struct {
	name              string
	mu                sync.Mutex
	RunCalls          []uint64 // track block numbers this executor ran on
	ApplyCacheCalls   [][]byte // track cached data applied
	hasValidOutput    bool
	hasOutputForFiles bool
	blockIndex        *index.BlockIndex
	outputData        []byte
	outputForFiles    []byte
	blockType         string
	moduleOutput      *pbssinternal.ModuleOutput
	runError          error
}

func NewTestingExecutor(name string, blockType string) *TestingExecutor {
	return &TestingExecutor{
		name:            name,
		blockType:       blockType,
		RunCalls:        []uint64{},
		ApplyCacheCalls: [][]byte{},
		hasValidOutput:  true,
	}
}

func (m *TestingExecutor) Name() string {
	return m.name
}

func (m *TestingExecutor) String() string {
	return fmt.Sprintf("testExecutor(%s)", m.name)
}

func (m *TestingExecutor) Close(ctx context.Context) error {
	return nil
}

func (m *TestingExecutor) run(ctx context.Context, reader execout.ExecutionOutputGetter, cachable bool) ([]byte, []byte, *pbssinternal.ModuleOutput, error) {
	clock := reader.Clock()
	m.mu.Lock()
	m.RunCalls = append(m.RunCalls, clock.Number)
	m.mu.Unlock()

	blk, _, err := reader.Get(m.blockType)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error on get blockType %q: %w", m.blockType, err)
	}

	// Create MD5 hash of the block content
	hash := md5.Sum(blk)
	blockHash := hex.EncodeToString(hash[:])

	if m.runError != nil {
		return nil, nil, nil, m.runError
	}

	// Generate output based on block number if not set
	outputData := m.outputData
	if outputData == nil {
		outputData = fmt.Appendf(nil, "output-%s-%d-%s", m.name, clock.Number, blockHash)
	}

	moduleOutput := m.moduleOutput
	if moduleOutput == nil {
		moduleOutput = &pbssinternal.ModuleOutput{
			ModuleName: m.name,
			Logs:       []string{fmt.Sprintf("executed at block %d", clock.Number)},
			Data: &pbssinternal.ModuleOutput_MapOutput{
				MapOutput: &anypb.Any{},
			},
		}
	}

	return outputData, m.outputForFiles, moduleOutput, nil
}

func (m *TestingExecutor) applyCachedOutput(value []byte) error {
	m.mu.Lock()
	m.ApplyCacheCalls = append(m.ApplyCacheCalls, value)
	m.mu.Unlock()
	return nil
}

func (m *TestingExecutor) toModuleOutput(data []byte) (*pbssinternal.ModuleOutput, error) {
	return &pbssinternal.ModuleOutput{
		ModuleName: m.name,
		Data: &pbssinternal.ModuleOutput_MapOutput{
			MapOutput: &anypb.Any{},
		},
	}, nil
}

func (m *TestingExecutor) HasValidOutput() bool {
	return m.hasValidOutput
}

func (m *TestingExecutor) SetHasValidOutput(value bool) {
	m.hasValidOutput = value
}

func (m *TestingExecutor) HasOutputForFiles() bool {
	return m.hasOutputForFiles
}

func (m *TestingExecutor) SetHasOutputForFiles(value bool) {
	m.hasOutputForFiles = value
}

func (m *TestingExecutor) BlockIndex() *index.BlockIndex {
	return m.blockIndex
}

func (m *TestingExecutor) RunsOnBlock(blockNum uint64) bool {
	return true
}

func (m *TestingExecutor) lastExecutionLogs() ([]string, bool) {
	if m.moduleOutput != nil {
		return m.moduleOutput.Logs, false
	}
	return nil, false
}

func (m *TestingExecutor) GetRunCalls() []uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]uint64{}, m.RunCalls...)
}

func (m *TestingExecutor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunCalls = []uint64{}
	m.ApplyCacheCalls = [][]byte{}
}

func (m *TestingExecutor) SetRunError(err error) {
	m.runError = err
}

func (m *TestingExecutor) SetOutputData(data []byte) {
	m.outputData = data
}

func (m *TestingExecutor) SetModuleOutput(output *pbssinternal.ModuleOutput) {
	m.moduleOutput = output
}

// Verify this implements ModuleExecutor at compile time
var _ ModuleExecutor = (*TestingExecutor)(nil)
