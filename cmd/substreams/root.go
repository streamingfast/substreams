package main

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "substreams",
	Short:        "A tool to manipulate Substreams, and process them locally and remotely",
	Long:         "Any place where <package> is specified, a 'substreams.yaml', a local '.spkg' file or an https://...spkg file can be specified",
	SilenceUsage: true,
}

func init() {
}
