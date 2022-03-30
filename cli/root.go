package cli

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "substreams",
	Short:        "A tool to manipulate Substreams, and process them locally and remotely",
	SilenceUsage: true,
}

func init() {
}
