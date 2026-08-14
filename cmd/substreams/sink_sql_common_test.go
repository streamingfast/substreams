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

// TestFromProtoConstraintPolicyDisableAll covers the deprecated --no-constraints, which
// has to keep meaning exactly what the three switches mean together.
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

	t.Run("the deprecated flag still means the same", func(t *testing.T) {
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("no-constraints", "true"))

		policy, err := fromProtoConstraintPolicy(cmd)
		require.NoError(t, err)
		assert.True(t, policy.SkipsEverything())
	})
}

// TestSetupCommandsCarryConstraintTiming pins that `setup` reads --apply-constraints: it
// builds its policy from that flag, so leaving it unregistered had setup read the empty
// string and create a bare schema whatever else was asked for.
func TestSetupCommandsCarryConstraintTiming(t *testing.T) {
	for _, cmd := range []*cobra.Command{sinkPostgresSetupCmd, sinkClickhouseSetupCmd} {
		assert.NotNil(t, cmd.Flags().Lookup("apply-constraints"), "%s must carry --apply-constraints", cmd.Use)
	}
}

// TestSetupOnlyCreatesConstraintsWhenAlways pins which of the three values `setup` acts on.
// It creates the schema and exits, so 'auto' and 'manual' both leave the tables bare and
// only 'always' puts the constraints on with them.
func TestSetupOnlyCreatesConstraintsWhenAlways(t *testing.T) {
	for value, wantUpfront := range map[string]bool{"auto": false, "manual": false, "always": true} {
		cmd := &cobra.Command{}
		addFromProtoSchemaFlags(cmd.Flags())
		addConstraintTimingFlag(cmd.Flags())
		require.NoError(t, cmd.Flags().Set("apply-constraints", value))

		policy, err := fromProtoConstraintPolicy(cmd)
		require.NoError(t, err)
		assert.Equal(t, wantUpfront, policy.ApplyUpfront(), "--apply-constraints=%s", value)
	}
}
