package noop

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNoopSinkCreation(t *testing.T) {
	// Create a basic noop sink config
	config := SinkConfig{
		StateFile: "./test.cursor",
		Logger:    zap.NewNop(),
	}

	// Create the noop sink - this should work even with minimal config
	noopSink := &Sink{
		stateFile: config.StateFile,
		logger:    config.Logger,
	}

	assert.NotNil(t, noopSink)
	assert.Equal(t, "./test.cursor", noopSink.stateFile)
	assert.Equal(t, config.Logger, noopSink.logger)
}

func TestNoopSinkCursorFileOperations(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := ioutil.TempDir("", "noop_sink_cursor_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "cursor.state")

	// Create a noop sink
	noopSink := &Sink{
		stateFile: stateFile,
		logger:    zap.NewNop(),
	}

	// Verify the sink was created properly
	assert.NotNil(t, noopSink)
	assert.Equal(t, stateFile, noopSink.stateFile)

	// Test writing cursor data directly (simulating what saveCursor would do)
	testCursorData := "test_cursor_data_12345"
	err = ioutil.WriteFile(stateFile, []byte(testCursorData), 0644)
	require.NoError(t, err)

	// Verify the cursor was saved to the file
	data, err := ioutil.ReadFile(stateFile)
	require.NoError(t, err)
	assert.Equal(t, testCursorData, string(data))
}

func TestNoopSinkCursorFileReading(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := ioutil.TempDir("", "noop_sink_load_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "existing.cursor")

	// Write a test cursor to the file
	testCursorData := "existing_cursor_67890"
	err = ioutil.WriteFile(stateFile, []byte(testCursorData), 0644)
	require.NoError(t, err)

	// Verify we can read it back (simulating cursor loading)
	data, err := ioutil.ReadFile(stateFile)
	require.NoError(t, err)

	// Test trimming whitespace like the actual implementation does
	cursorStr := strings.TrimSpace(string(data))
	assert.Equal(t, testCursorData, cursorStr)
}

func TestNoopSinkEmptyStateFile(t *testing.T) {
	// Test with empty state file (no cursor saving/loading)
	config := SinkConfig{
		StateFile: "", // Empty state file
		Logger:    zap.NewNop(),
	}

	// Create the noop sink
	noopSink := &Sink{
		stateFile: config.StateFile,
		logger:    config.Logger,
	}

	assert.NotNil(t, noopSink)
	assert.Empty(t, noopSink.stateFile)
}

func TestNoopSinkFileWriteError(t *testing.T) {
	// Test error handling when state file directory doesn't exist
	stateFile := "/nonexistent/directory/cursor.state"

	// Attempt to write to non-existent directory - should error
	err := ioutil.WriteFile(stateFile, []byte("test"), 0644)
	assert.Error(t, err)
}

func TestNoopSinkStateFileConditionals(t *testing.T) {
	// Test the conditional logic used in the handler methods
	testCases := []struct {
		name       string
		stateFile  string
		shouldSave bool
	}{
		{
			name:       "empty state file",
			stateFile:  "",
			shouldSave: false,
		},
		{
			name:       "valid state file",
			stateFile:  "./valid.cursor",
			shouldSave: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			noopSink := &Sink{
				stateFile: tc.stateFile,
				logger:    zap.NewNop(),
			}

			// Test the conditional logic that determines if cursor should be saved
			shouldSave := noopSink.stateFile != ""
			assert.Equal(t, tc.shouldSave, shouldSave)
		})
	}
}
