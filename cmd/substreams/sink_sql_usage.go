package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// setModeGroupedUsage partitions the run command's flags by the mode they apply to.
//
// Both vocabularies have to be registered on the same command: the mode is read from the
// module's output type at run time, long after init() has registered anything. So the run
// command lists knobs that are inert for any given Substreams, and a per-line `[mode]`
// prefix is left as the only thing separating them — which the operator has to scan for.
//
// Headings do that job better, and free every help string of its prefix.
func setModeGroupedUsage(cmd *cobra.Command) {
	groups := []flagGroup{
		{
			title: "From-proto mode flags (module outputs an arbitrary protobuf message)",
			names: append(append([]string{}, fromProtoSchemaFlagNames...), fromProtoRunFlagNames...),
		},
		{
			title: "DatabaseChanges mode flags (module outputs DatabaseChanges)",
			names: append(databaseChangesFlagNames, onModuleHashMismatchFlag),
		},
	}

	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		cmd.OutOrStderr().Write([]byte(groupedUsage(cmd, groups))) //nolint:errcheck // usage output

		return nil
	})
}

type flagGroup struct {
	title string
	names []string
}

func groupedUsage(cmd *cobra.Command, groups []flagGroup) string {
	var out strings.Builder

	fmt.Fprintf(&out, "Usage:\n  %s\n", cmd.UseLine())
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(&out, "\nAvailable Commands:\n")
		for _, sub := range cmd.Commands() {
			if sub.IsAvailableCommand() {
				fmt.Fprintf(&out, "  %-16s %s\n", sub.Name(), sub.Short)
			}
		}
	}

	claimed := map[string]bool{}
	for _, group := range groups {
		set := pflag.NewFlagSet(group.title, pflag.ContinueOnError)
		for _, name := range group.names {
			if flag := cmd.Flags().Lookup(name); flag != nil && !flag.Hidden {
				set.AddFlag(flag)
				claimed[name] = true
			}
		}
		if !set.HasAvailableFlags() {
			continue
		}

		fmt.Fprintf(&out, "\n%s:\n%s", group.title, set.FlagUsages())
	}

	common := pflag.NewFlagSet("common", pflag.ContinueOnError)
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if !claimed[flag.Name] && !flag.Hidden {
			common.AddFlag(flag)
		}
	})
	if common.HasAvailableFlags() {
		fmt.Fprintf(&out, "\nFlags (both modes):\n%s", common.FlagUsages())
	}

	if inherited := cmd.InheritedFlags(); inherited.HasAvailableFlags() {
		fmt.Fprintf(&out, "\nGlobal Flags:\n%s", inherited.FlagUsages())
	}

	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(&out, "\nUse \"%s [command] --help\" for more information about a command.\n", cmd.CommandPath())
	}

	return out.String()
}
