package tools

import "github.com/spf13/cobra"

var analyticsCmd = &cobra.Command{
	Use:          "analytics",
	SilenceUsage: true,
}

func init() {
	Cmd.AddCommand(analyticsCmd)
}
