package main

import (
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "substreams",
	Short:        "A tool to manipulate Substreams, and process them locally and remotely",
	Long:         "Any place where <package> is specified, a 'substreams.yaml', a local '.spkg' file or an https://...spkg file can be specified",
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		setup(cmd, zapcore.WarnLevel)
	},
}

func init() {
	// From https://thegraph.com/docs/en/operating-graph-node/
	rootCmd.PersistentFlags().String("ipfs-url", "https://ipfs.network.thegraph.com", "IPFS endpoint to resolve substreams-based subgraphs as manifest")
	rootCmd.PersistentFlags().Duration("ipfs-timeout", time.Second*10, "IPFS timeout when resolving substreams-based subgraphs as manifest")
}
