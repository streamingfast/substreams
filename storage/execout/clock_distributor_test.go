package execout

import (
	"context"
	"io"
	"testing"

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MockFileReader is a mock implementation of FileReader for testing
type MockFileReader struct {
	items      []*pboutput.Item
	currentIdx int
	moduleName string
	filename   string
	readError  error
}

func (m *MockFileReader) ReadNext() (*pboutput.Item, error) {
	if m.readError != nil {
		return nil, m.readError
	}

	if m.currentIdx >= len(m.items) {
		return nil, io.EOF
	}

	item := m.items[m.currentIdx]
	m.currentIdx++
	return item, nil
}

func (m *MockFileReader) Get(ctx context.Context, blockNumber uint64) (payload []byte, found bool, err error) {
	// Simple implementation for testing
	return nil, false, nil
}

func (m *MockFileReader) ModuleName() string {
	return m.moduleName
}

func (m *MockFileReader) Filename() string {
	return m.filename
}

func (m *MockFileReader) SetItems(items []*pboutput.Item) {
	m.items = items
	m.currentIdx = 0
}

func (m *MockFileReader) SetReadError(err error) {
	m.readError = err
}

func TestNewClockDistributor(t *testing.T) {
	tests := []struct {
		name       string
		execOuts   map[string]FileReader
		startBlock uint64
		stopBlock  uint64
	}{
		{
			name:       "empty exec outs",
			execOuts:   make(map[string]FileReader),
			startBlock: 100,
			stopBlock:  200,
		},
		{
			name: "single exec out",
			execOuts: map[string]FileReader{
				"module1": &MockFileReader{moduleName: "module1", filename: "file1.output"},
			},
			startBlock: 100,
			stopBlock:  200,
		},
		{
			name: "multiple exec outs",
			execOuts: map[string]FileReader{
				"module1": &MockFileReader{moduleName: "module1", filename: "file1.output"},
				"module2": &MockFileReader{moduleName: "module2", filename: "file2.output"},
			},
			startBlock: 100,
			stopBlock:  200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := NewClockDistributor(tt.execOuts, tt.startBlock, tt.stopBlock)

			assert.NotNil(t, cd)
			assert.Equal(t, tt.execOuts, cd.execOuts)
			assert.Equal(t, tt.startBlock, cd.startBlock)
			assert.Equal(t, tt.stopBlock, cd.stopBlock)
			assert.Equal(t, tt.startBlock, cd.nextClockNumber)
			assert.NotNil(t, cd.execOutsLastclock)
			assert.NotNil(t, cd.seenClocks)
			assert.Empty(t, cd.execOutsLastclock)
			assert.Empty(t, cd.seenClocks)
		})
	}
}
func TestClockDistributor_Next_MultipleModules(t *testing.T) {
	mockReader1 := &MockFileReader{
		moduleName: "module1",
		filename:   "module1.output",
	}
	mockReader2 := &MockFileReader{
		moduleName: "module2",
		filename:   "module2.output",
	}

	// Set up items for module1 - has blocks 100, 102
	items1 := []*pboutput.Item{
		{
			BlockNum:  100,
			BlockId:   "block_100",
			Timestamp: timestamppb.Now(),
		},
		{
			BlockNum:  102,
			BlockId:   "block_102",
			Timestamp: timestamppb.Now(),
		},
		{
			BlockNum:  107,
			BlockId:   "block_102",
			Timestamp: timestamppb.Now(),
		},
	}
	mockReader1.SetItems(items1)

	// Set up items for module2 - has blocks 101, 102
	items2 := []*pboutput.Item{
		{
			BlockNum:  101,
			BlockId:   "block_101",
			Timestamp: timestamppb.Now(),
		},
		{
			BlockNum:  102,
			BlockId:   "block_102",
			Timestamp: timestamppb.Now(),
		},
		{
			BlockNum:  105,
			BlockId:   "block_105",
			Timestamp: timestamppb.Now(),
		},
	}
	mockReader2.SetItems(items2)

	execOuts := map[string]FileReader{
		"module1": mockReader1,
		"module2": mockReader2,
	}

	cd := NewClockDistributor(execOuts, 100, 106)
	ctx := context.Background()

	clock, err := cd.Next(ctx)
	require.NoError(t, err)
	assert.NotNil(t, clock)
	assert.Equal(t, uint64(100), clock.Number)
	assert.Equal(t, "block_100", clock.Id)
	clock, err = cd.Next(ctx)
	require.NoError(t, err)
	assert.NotNil(t, clock)
	assert.Equal(t, uint64(101), clock.Number)
	assert.Equal(t, "block_101", clock.Id)

	clock, err = cd.Next(ctx)
	require.NoError(t, err)
	assert.NotNil(t, clock)
	assert.Equal(t, uint64(102), clock.Number)
	assert.Equal(t, "block_102", clock.Id)

	clock, err = cd.Next(ctx)
	require.NoError(t, err)
	assert.NotNil(t, clock)
	assert.Equal(t, uint64(105), clock.Number)
	assert.Equal(t, "block_105", clock.Id)

	clock, err = cd.Next(ctx)
	assert.Equal(t, io.EOF, err)
	assert.Nil(t, clock)
}
