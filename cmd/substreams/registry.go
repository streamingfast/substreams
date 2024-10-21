package main

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage substreams registry",
	Long: cli.Dedent(`
		Login, publish and list packages from the Substreams registry
	`),
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(registryCmd)
}
