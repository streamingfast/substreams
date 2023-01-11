package main

import "github.com/spf13/cobra"

var alphaCmd = &cobra.Command{
	Use:          "alpha",
	Short:        "Group of commands that are currently being available for testing but could change at any time",
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(alphaCmd)
}
