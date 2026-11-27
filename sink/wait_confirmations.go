package sink

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/cli/sflags"
	"go.uber.org/zap"
)

// WaitConfirmationsUsage is the help text of the [FlagWaitConfirmations] flag.
const WaitConfirmationsUsage = "Hold the most recent reversible blocks back from the sink until they are final, or until N newer blocks " +
	"have arrived, in which case they are assumed final. A reorganization that fits within the N blocks is absorbed silently " +
	"and never reaches the sink, a deeper one is a fatal error. Costs N blocks of latency near head, and the stop block is " +
	"pushed by N so the last blocks can be released. 0 disables it. Forced to 0 with --final-blocks-only"

// AddWaitConfirmationsFlags registers [FlagWaitConfirmations] with the given usage and its
// deprecated hidden alias [FlagUndoBufferSize]. Sinks that need their own help text for the
// flag use it instead of [AddFlagsToSet]; read the value back with [ResolveWaitConfirmations].
func AddWaitConfirmationsFlags(flags *pflag.FlagSet, usage string) {
	flags.Int(FlagWaitConfirmations, 0, usage)
	flags.Int(FlagUndoBufferSize, 0, "Deprecated: use --"+FlagWaitConfirmations)
	_ = flags.MarkHidden(FlagUndoBufferSize)
}

// ResolveWaitConfirmations returns the value of [FlagWaitConfirmations], falling back to its
// deprecated alias [FlagUndoBufferSize] (with a warning) when only that one was set. Setting both
// is an error.
func ResolveWaitConfirmations(cmd *cobra.Command, logger *zap.Logger) (int, error) {
	hasNew := sflags.FlagDefined(cmd, FlagWaitConfirmations)
	hasOld := sflags.FlagDefined(cmd, FlagUndoBufferSize)

	newChanged := hasNew && cmd.Flags().Changed(FlagWaitConfirmations)
	oldChanged := hasOld && cmd.Flags().Changed(FlagUndoBufferSize)

	switch {
	case newChanged && oldChanged:
		return 0, fmt.Errorf("flags --%s and --%s cannot be used together, --%s is its deprecated name", FlagWaitConfirmations, FlagUndoBufferSize, FlagUndoBufferSize)
	case oldChanged:
		logger.Warn("flag --" + FlagUndoBufferSize + " is deprecated, use --" + FlagWaitConfirmations + " instead")
		return sflags.MustGetInt(cmd, FlagUndoBufferSize), nil
	case hasNew:
		return sflags.MustGetInt(cmd, FlagWaitConfirmations), nil
	}

	return 0, nil
}

// effectiveWaitConfirmations merges [SinkerConfig.WaitConfirmations] with the deprecated
// [SinkerConfig.UndoBufferSize]: the deprecated field only applies when the new one is unset.
func (c *SinkerConfig) effectiveWaitConfirmations() (int, error) {
	if c.UndoBufferSize != 0 && c.WaitConfirmations != 0 && c.UndoBufferSize != c.WaitConfirmations {
		return 0, fmt.Errorf("SinkerConfig.WaitConfirmations (%d) and its deprecated alias SinkerConfig.UndoBufferSize (%d) are both set to different values", c.WaitConfirmations, c.UndoBufferSize)
	}

	if c.WaitConfirmations != 0 {
		return c.WaitConfirmations, nil
	}

	return c.UndoBufferSize, nil
}
