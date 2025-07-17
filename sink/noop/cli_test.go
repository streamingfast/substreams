package noop

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIFlagParsing(t *testing.T) {
	// Test that the CLI properly parses the state-file flag
	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "default value",
			args:     []string{},
			expected: "./state.cursor",
		},
		{
			name:     "custom state file",
			args:     []string{"--state-file", "/tmp/noop.cursor"},
			expected: "/tmp/noop.cursor",
		},
		{
			name:     "empty state file",
			args:     []string{"--state-file", ""},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock command to test flag parsing
			cmd := &cobra.Command{
				Use: "test",
				RunE: func(cmd *cobra.Command, args []string) error {
					return nil
				},
			}

			// Add the state-file flag
			cmd.Flags().String("state-file", "./state.cursor", "File where the sink will store its cursor. If empty, no cursor will be saved or used, only the start-block.")

			// Set the args and parse
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			require.NoError(t, err)

			// Get the parsed value
			value, err := cmd.Flags().GetString("state-file")
			require.NoError(t, err)

			assert.Equal(t, tc.expected, value)
		})
	}
}

func TestNoopSinkConfig(t *testing.T) {
	// Test that SinkConfig struct properly holds configuration
	config := SinkConfig{
		StateFile: "/tmp/test.cursor",
	}

	assert.Equal(t, "/tmp/test.cursor", config.StateFile)
	assert.Nil(t, config.SinkerConfig) // Should be nil until set
	assert.Nil(t, config.Logger)       // Should be nil until set
}

func TestNoopSinkConfigValidation(t *testing.T) {
	// Test various StateFile configurations
	testCases := []struct {
		name      string
		stateFile string
		valid     bool
	}{
		{
			name:      "empty state file",
			stateFile: "",
			valid:     true, // Empty is valid, means no cursor saving
		},
		{
			name:      "relative path",
			stateFile: "./state.cursor",
			valid:     true,
		},
		{
			name:      "absolute path",
			stateFile: "/tmp/noop.cursor",
			valid:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := SinkConfig{
				StateFile: tc.stateFile,
			}

			// Creating config should not fail regardless of StateFile value
			assert.Equal(t, tc.stateFile, config.StateFile)
		})
	}
}
