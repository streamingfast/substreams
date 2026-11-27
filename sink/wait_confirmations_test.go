package sink

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newWaitConfirmationsCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
	AddFlagsToSet(cmd.Flags())
	require.NoError(t, cmd.Flags().Parse(args))

	return cmd
}

func TestResolveWaitConfirmations(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expected     int
		expectWarn   bool
		expectErrMsg string
	}{
		{name: "unset", args: nil, expected: 0},
		{name: "new flag", args: []string{"--wait-confirmations=12"}, expected: 12},
		{name: "deprecated alias", args: []string{"--undo-buffer-size=7"}, expected: 7, expectWarn: true},
		{name: "both", args: []string{"--wait-confirmations=12", "--undo-buffer-size=7"}, expectErrMsg: "cannot be used together"},
		{name: "both same value", args: []string{"--wait-confirmations=7", "--undo-buffer-size=7"}, expectErrMsg: "cannot be used together"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)
			cmd := newWaitConfirmationsCmd(t, tt.args...)

			actual, err := ResolveWaitConfirmations(cmd, zap.New(core))
			if tt.expectErrMsg != "" {
				require.ErrorContains(t, err, tt.expectErrMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.expectWarn, logs.Len() == 1)
		})
	}
}

func TestDeprecatedAliasIsHidden(t *testing.T) {
	cmd := newWaitConfirmationsCmd(t)

	assert.True(t, cmd.Flags().Lookup(FlagUndoBufferSize).Hidden)
	assert.False(t, cmd.Flags().Lookup(FlagWaitConfirmations).Hidden)
}

func TestEffectiveWaitConfirmations(t *testing.T) {
	tests := []struct {
		name         string
		config       SinkerConfig
		expected     int
		expectErrMsg string
	}{
		{name: "unset", config: SinkerConfig{}, expected: 0},
		{name: "new field", config: SinkerConfig{WaitConfirmations: 5}, expected: 5},
		{name: "deprecated field", config: SinkerConfig{UndoBufferSize: 6}, expected: 6},
		{name: "both equal", config: SinkerConfig{WaitConfirmations: 5, UndoBufferSize: 5}, expected: 5},
		{name: "both different", config: SinkerConfig{WaitConfirmations: 5, UndoBufferSize: 6}, expectErrMsg: "both set"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := tt.config.effectiveWaitConfirmations()
			if tt.expectErrMsg != "" {
				require.ErrorContains(t, err, tt.expectErrMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
