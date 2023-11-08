package main

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
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
	rootCmd.PersistentFlags().Bool("skip-package-validation", false, "Do not perform any validation when reading substreams package")
}

func getReaderOpts(cmd *cobra.Command) (out []manifest.Option) {
	if sflags.MustGetBool(cmd, "skip-package-validation") {
		out = append(out, manifest.SkipPackageValidationReader())
	} else {
	}
	return
}
