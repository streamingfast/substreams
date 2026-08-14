package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSinkUserAgent pins what the server sees: the two engines run the same commands, so
// the mode alone did not say where the rows went.
func TestSinkUserAgent(t *testing.T) {
	assert.Equal(t, "sink_from_proto_ch", sinkUserAgent("sink_from_proto", sinkClickhouseDriver))
	assert.Equal(t, "sink_from_proto_pg", sinkUserAgent("sink_from_proto", sinkPostgresDriver))
	assert.Equal(t, "sink_database_changes_ch", sinkUserAgent("sink_database_changes", sinkClickhouseDriver))
	assert.Equal(t, "sink_database_changes_pg", sinkUserAgent("sink_database_changes", sinkPostgresDriver))
	assert.Equal(t, "sink_from_proto", sinkUserAgent("sink_from_proto", "duckdb"))
}

// TestFromProtoConstraintPolicyDisableAll covers the shorthand and the deprecated flag it
// replaces, both of which have to mean exactly what the three switches mean together.
func TestFromProtoConstraintPolicyDisableAll(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		addFromProtoSchemaFlags(cmd.Flags())
		addConstraintTimingFlag(cmd.Flags())

		return cmd
	}

	t.Run("nothing disabled by default", func(t *testing.T) {
		policy, err := fromProtoConstraintPolicy(newCmd())
		require.NoError(t, err)
		assert.False(t, policy.SkipsEverything())
	})

	t.Run("the shorthand disables all three", func(t *testing.T) {
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("disable-all-constraints", "true"))

		policy, err := fromProtoConstraintPolicy(cmd)
		require.NoError(t, err)
		assert.True(t, policy.SkipsEverything())
	})

	t.Run("the deprecated flag still means the same", func(t *testing.T) {
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("no-constraints", "true"))

		policy, err := fromProtoConstraintPolicy(cmd)
		require.NoError(t, err)
		assert.True(t, policy.SkipsEverything())
	})
}

// TestSetupCommandsCarryNoConstraintTiming pins that --apply-constraints stays off `setup`:
// there the question is whether the schema comes out constrained, not when, and two of the
// flag's three values would collapse onto the same answer.
func TestSetupCommandsCarryNoConstraintTiming(t *testing.T) {
	for _, cmd := range []*cobra.Command{sinkPostgresSetupCmd, sinkClickhouseSetupCmd} {
		assert.Nil(t, cmd.Flags().Lookup("apply-constraints"), "%s must not carry --apply-constraints", cmd.Use)
		assert.NotNil(t, cmd.Flags().Lookup("disable-all-constraints"), "%s must carry --disable-all-constraints", cmd.Use)
	}
}
