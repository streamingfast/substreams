package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	cobra.OnInitialize(func() {
		autoBind(rootCmd, "SUBSTREAMS")
	})
}

func autoBind(root *cobra.Command, prefix string) {
	recurseCommands(root, prefix, nil) // []string{strings.ToLower(prefix)}) how does it wweeeerrkk?
}

func recurseCommands(root *cobra.Command, prefix string, segments []string) {
	var segmentPrefix string
	if len(segments) > 0 {
		segmentPrefix = strings.ToUpper(strings.Join(segments, "_")) + "_"
	}

	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		newName := strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
		varName := prefix + "_" + segmentPrefix + "GLOBAL_" + newName
		if val := os.Getenv(varName); val != "" {
			f.Usage += " [LOADED FROM ENV]" // Until we have a better template for our usage.
			if !f.Changed {
				f.Value.Set(val)
			}
		}
	})

	root.Flags().VisitAll(func(f *pflag.Flag) {
		newName := strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
		varName := prefix + "_" + segmentPrefix + "CMD_" + newName
		if val := os.Getenv(varName); val != "" {
			f.Usage += " [LOADED FROM ENV]"
			if !f.Changed {
				f.Value.Set(val)
			}
		}
	})

	for _, cmd := range root.Commands() {
		recurseCommands(cmd, prefix, append(segments, cmd.Name()))
	}
}

func mustGetString(cmd *cobra.Command, flagName string) string {
	val, err := cmd.Flags().GetString(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}

func mustGetStringArray(cmd *cobra.Command, flagName string) []string {
	val, err := cmd.Flags().GetStringArray(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}
func mustGetStringSlice(cmd *cobra.Command, flagName string) []string {
	val, err := cmd.Flags().GetStringSlice(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	if len(val) == 0 {
		return nil
	}
	return val
}
func mustGetInt64(cmd *cobra.Command, flagName string) int64 {
	val, err := cmd.Flags().GetInt64(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}
func mustGetUint64(cmd *cobra.Command, flagName string) uint64 {
	val, err := cmd.Flags().GetUint64(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}
func mustGetBool(cmd *cobra.Command, flagName string) bool {
	val, err := cmd.Flags().GetBool(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}

func maybeGetString(cmd *cobra.Command, flagName string) string {
	val, _ := cmd.Flags().GetString(flagName)
	return val
}
func maybeGetInt64(cmd *cobra.Command, flagName string) int64 {
	val, _ := cmd.Flags().GetInt64(flagName)
	return val
}
func maybeGetUint64(cmd *cobra.Command, flagName string) uint64 {
	val, _ := cmd.Flags().GetUint64(flagName)
	return val
}
func maybeGetBool(cmd *cobra.Command, flagName string) bool {
	val, _ := cmd.Flags().GetBool(flagName)
	return val
}
