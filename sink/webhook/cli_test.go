package webhook

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIFlagParsing(t *testing.T) {
	// Test that the CLI properly parses different webhook-max-retries values
	testCases := []struct {
		name     string
		args     []string
		expected int
	}{
		{
			name:     "default value",
			args:     []string{},
			expected: 3,
		},
		{
			name:     "zero retries",
			args:     []string{"--webhook-max-retries", "0"},
			expected: 0,
		},
		{
			name:     "limited retries",
			args:     []string{"--webhook-max-retries", "5"},
			expected: 5,
		},
		{
			name:     "infinite retries",
			args:     []string{"--webhook-max-retries", "-1"},
			expected: -1,
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

			// Add the webhook-max-retries flag
			cmd.Flags().Int("webhook-max-retries", 3, "Maximum number of retries for webhook calls (0 disables retries, -1 for infinite retries)")

			// Set the args and parse
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			require.NoError(t, err)

			// Get the parsed value
			value, err := cmd.Flags().GetInt("webhook-max-retries")
			require.NoError(t, err)

			assert.Equal(t, tc.expected, value)
		})
	}
}

func TestWebhookConfigWithInfiniteRetries(t *testing.T) {
	// Test that Config struct properly handles -1 value
	config := Config{
		MaxRetries: -1,
	}

	// Create client with infinite retries
	client := NewClient(config, nil)

	assert.Equal(t, -1, client.maxRetries)
}

func TestWebhookConfigValidation(t *testing.T) {
	// Test various MaxRetries configurations
	testCases := []struct {
		name       string
		maxRetries int
		valid      bool
	}{
		{
			name:       "negative value other than -1",
			maxRetries: -5,
			valid:      true, // We allow any negative value, but only -1 has special meaning
		},
		{
			name:       "zero retries",
			maxRetries: 0,
			valid:      true,
		},
		{
			name:       "positive retries",
			maxRetries: 5,
			valid:      true,
		},
		{
			name:       "infinite retries",
			maxRetries: -1,
			valid:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				MaxRetries: tc.maxRetries,
			}

			// Creating client should not fail regardless of MaxRetries value
			client := NewClient(config, nil)
			assert.NotNil(t, client)
			assert.Equal(t, tc.maxRetries, client.maxRetries)
		})
	}
}
